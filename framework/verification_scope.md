# Verification Scope

## Problem

`validate` and `verify` operate on a spec unit against the relevant codebase or document set. The older scoped/full dual-mode design used git working-directory changes to pick a subset. Practice showed that subset results are structurally unusable: the promote gate only accepts results from a complete run, so a scoped result always had to be re-run in full before promote — the scoped pass was wasted work. The subset also did not match what the user actually cares about: git diff reflects "what was recently changed", not "what the user is worried about".

The one reason for scoped mode was context saturation on large codebases. That problem is solved structurally by the coverage model (see §Coverage Model below) — a run decomposes into agent-chosen, independently completable reviewer sessions instead of shrinking the checked surface.

## Solution

There are two complete-coverage run modes: **full** (`validate@{target}`, `verify@{unit}`) and **delta** (`revalidate@{target}`, `reverify@{unit}`). A full run checks everything in scope. A delta run re-checks the judgments whose dependency evidence went stale and carries the rest over — the result is a complete check either way (see §Delta Runs).

Targeted checking exists only through explicit user choice: `:check-{n}` and `:{keyword}`. The user declares the focus, not git diff. Targeted runs are for iterative feedback — they never write a cache, so they can never satisfy the promote gate.

**Cache invariant:** only a complete-coverage run (full or delta) writes a cache. A cache exists means a complete check passed . Targeted runs report findings and write nothing.

**Layer roles:** the verification loop — caches, promote eligibility, and delta re-runs — belongs to the candidate layer. The stable layer has no gate; applicable full commands run against a stable-only target as **confirmation checks** (unit: validate/verify; rule: validate). They write `target: stable` caches consumed by `fresh@stable` and never edit truth. Normal design changes first fork stable to candidate (`framework/concepts.md`). Delta re-runs restore a stale confirmation cache when a usable pass baseline exists (see §Delta Runs → Layer applicability).

## Principles

1. **Full by default, complete always** — every command without a `:` suffix runs the complete check; delta runs (`re*`) also produce a complete check by re-checking the stale part and carrying the rest over. No git-awareness, no subset selection.
2. **Targeted only on explicit user choice** — `:check-{n}` (validate) and `:{keyword}` (both commands) scope the run to a user-declared focus.
3. **Targeted results never satisfy promote** — targeted runs never publish a complete result cache. A targeted P0/P1 records invalidation through `gate-invalidate`: it invalidates contradicted immutable verify evidence and transitions the gate cache and matching open runs. Only a complete-coverage run's cache passes the promote gate.
4. **Delta only on explicit user choice** — `revalidate@{target}` / `reverify@{unit}` re-run only the judgments whose evidence went stale and write a cache with `basis: delta`. The incremental scope is derived from the stale cache evidence and reported to the user explicitly (see §Delta Runs).
5. **Delta restores both layers** — the incremental re-run restores promote eligibility for candidates and a stale confirmation state for stable-only targets (whose confirmation cache exists with `result: pass`); a failure record (BLOCKED cache) is restored by the failure-recovery delta run (`basis: repair`). A delta run against a stable-only target without a usable baseline reports "No usable confirmation baseline. Run the full `{command}@{target}` (confirmation check) first, or `specflowctl fork {--unit|--rule} {name}` to start a new round." (see §Delta Runs → Layer applicability).
6. **Keyword means "what the user wants to look at"** — each command resolves the keyword inside its own target domain (see Keyword Resolution).
7. **No fixed item definition** — the framework does not define what an "item" is. The agent reads the spec structure dynamically.

## Syntax

### Validate

| User says | What agent does |
|-----------|-----------------|
| `validate@{target}` | Full: all 10 checks. Candidate target: writes the validate cache (`mode: full`, promote gate); on FAIL writes a failure record with the per-check `status` map — the `revalidate@{target}` failure-recovery baseline (see §Failure handling by gate role in `framework/validation_cache.md`). Stable-only target (no candidate file): the same 10 checks against the stable content and its current dependencies and rules — writes the cache with `target: stable` (confirmation state consumed by `fresh@stable`); on FAIL writes a failure record and recommends forking (see §Stable-only Targets). Writes via the coverage-run sequence (`specflowctl gate-plan` before the executor reads any input, `gate-mission --keys ...` per agent-chosen batch with its output sent verbatim as that reviewer's mission, one `specflowctl gate-submit` per session report, and — when relationships are assigned or findings exist — one `gate-mission --final` final synthesis, then `specflowctl gate-finalize` after every coverage key is covered — `specflow/tooling/bin/specflowctl`) — see §Coverage model and `framework/validation_cache.md` §Write Rules (Tooled writes). Main agent MUST plan the gate run before the executor reads any input and finalize it only after every coverage key is covered; otherwise `fresh` stays `MISSING` (or `BLOCKED` for a failure record) and `promote` is rejected — see `framework/unit_validate_checklist.md` §Completion — Persist Gate Cache. |
| `validate@{target}:check-{n}` | Targeted: single check `{n}` only. User explicitly chooses focus. Does not write a cache. Targeted runs intentionally do NOT write a cache; completion is the report alone (see `framework/unit_validate_checklist.md` §Completion — Persist Gate Cache). |
| `validate@{target}:{keyword}` | Targeted: matches keyword to a check name (e.g., "design" → Check 2, "scope" → Check 3). Does not write a cache. Targeted runs intentionally do NOT write a cache; completion is the report alone (see `framework/unit_validate_checklist.md` §Completion — Persist Gate Cache). |

### Verify

| User says | What agent does |
|-----------|-----------------|
| `verify@{unit}` | Full: verify all spec content and the declared code surface — one gate with two lenses: `alignment` (all 7 steps per acceptance item) and `quality` (public code and unit design per file, plus one unit architecture task). Candidate target: writes the verify cache (`mode: full`, promote gate) carrying the merged `alignment` + `quality` sections; on FAIL (P0/P1) writes a failure record with the per-key `status` map — the `reverify@{unit}` failure-recovery baseline (see §Failure handling by gate role in `framework/validation_cache.md`). Stable-only target (no candidate file): verify the stable spec against the code (drift confirmation) — writes the cache with `target: stable` (VERIFIED state consumed by `fresh@stable`); on MISMATCH writes a failure record, reports the drift, and recommends forking (see §Stable-only Targets). Writes via the coverage-run sequence (`specflowctl gate-plan` before the executor reads any input, `gate-mission --keys ...` per agent-chosen batch with the same kind and lens with its output sent verbatim as that reviewer's mission, one `specflowctl gate-submit` per session report, and — when relationships are assigned or findings exist — one `gate-mission --final` final synthesis, then `specflowctl gate-finalize` after every coverage key is covered — `specflow/tooling/bin/specflowctl`) — see §Coverage model and `framework/validation_cache.md` §Write Rules (Tooled writes). Main agent MUST plan the gate run before the executor reads any input and finalize it only after every coverage key is covered; otherwise `fresh` stays `MISSING` (or `BLOCKED` for a failure record) and `promote` is rejected — see `framework/unit_verify_checklist.md` §Completion — Persist Gate Cache. |
| `verify@{unit}:{keyword}` | Targeted: matches keyword to spec content (section title, feature name, API path, etc.) → verify that content. Does not write a cache. Targeted runs intentionally do NOT write a cache; completion is the report alone (see `framework/unit_verify_checklist.md` §Completion — Persist Gate Cache). |

### Rule (validate only, verify removed)

| User says | What agent does |
|-----------|-----------------|
| `validate@{rule}` | Full: every current check in `framework/rule_validate_checklist.md`. Writes the validate cache (`mode: full`, promote gate) via the coverage-run sequence (`specflowctl gate-plan` before execution, `gate-mission --keys ...` for the agent-chosen batch with its output sent verbatim as the reviewer's mission, one `specflowctl gate-submit` for the single session report, `specflowctl gate-finalize` after every coverage key is covered; `specflow/tooling/bin/specflowctl`) — see §Coverage model and `framework/validation_cache.md` §Write Rules. Main agent MUST plan the gate run before the executor reads any input and finalize it only after every coverage key is covered; otherwise `fresh` stays `MISSING` (or `BLOCKED` for a failure record) and `promote` is rejected — see `framework/rule_validate_checklist.md` §Completion — Persist Gate Cache. |
| `validate@{rule}:check-{n}` | Targeted: single check `{n}` only. User explicitly chooses focus. Does not write a cache. Targeted runs intentionally do NOT write a cache; completion is the report alone (see `framework/rule_validate_checklist.md` §Completion — Persist Gate Cache). |
| `validate@{rule}:{keyword}` | Targeted: matches keyword to a check name. Does not write a cache. Targeted runs intentionally do NOT write a cache; completion is the report alone (see `framework/rule_validate_checklist.md` §Completion — Persist Gate Cache). |

> `verify` on a Rule target has been removed. If the user says `verify@{rule}`, report: "Rule verify has been removed. Run `validate@{rule}` instead." See `framework/concepts.md` for context.

### Delta re-runs (re* instruction family)

| User says | What agent does |
|-----------|-----------------|
| `revalidate@{target}` | Delta: re-run only the checks whose dependency evidence went stale, carry the rest over; a failure-record baseline re-runs the failed checks instead (see §Delta Runs → Failure recovery). Writes a cache with `mode: full`, `basis: delta` (`repair` from a failure record); delta FAIL writes a failure record. Writes via the coverage-run sequence (`specflowctl gate-plan --mode delta` (or `--mode repair` from a failure record) before the re-run, `gate-mission --keys ...` per batch with its output sent verbatim as the reviewer's mission, one `specflowctl gate-submit` per session report, and — when relationships are assigned or findings exist — one `gate-mission --final`, then `specflowctl gate-finalize` — `specflow/tooling/bin/specflowctl`) — see §Coverage model and `framework/validation_cache.md` §Write Rules (Tooled writes). Candidate targets, plus stable-only targets with a usable baseline (see §Delta Runs → Layer applicability). Preconditions and scope rules in §Delta Runs. Main agent MUST plan the re-run before it starts and finalize it only after every coverage key is covered; otherwise `fresh` stays `MISSING`/`BLOCKED` and `promote` is rejected — see the target-appropriate checklist §Completion — Persist Gate Cache. |
| `reverify@{unit}` | Delta: re-verify only the spec content and code whose evidence went stale, carry the rest over; a failure-record baseline re-runs the failed judgments instead. Writes a cache with `mode: full`, `basis: delta` (`repair` from a failure record); delta FAIL writes a failure record. Writes via the coverage-run sequence (`specflowctl gate-plan --mode delta` (or `--mode repair` from a failure record) before the re-run, `gate-mission --keys ...` per batch with the same kind and lens, one `specflowctl gate-submit` per session report, and — when relationships are assigned or findings exist — one `gate-mission --final`, then `specflowctl gate-finalize` — `specflow/tooling/bin/specflowctl`) — see §Coverage model and `framework/validation_cache.md` §Write Rules (Tooled writes). Candidate targets, plus stable-only targets with a usable baseline (rule verify has been removed). Main agent MUST plan the re-run before it starts and finalize it only after every coverage key is covered; otherwise `fresh` stays `MISSING`/`BLOCKED` and `promote` is rejected — see `framework/unit_verify_checklist.md` §Completion — Persist Gate Cache. |

No `:keyword` / `:check-{n}` variant — a delta run is a complete-coverage run, not a targeted one.

### Freshness check (read-only)

| User says | What agent does |
|-----------|-----------------|
| `fresh@{target}` | Read-only report of the target's cache freshness. Runs `specflowctl fresh --unit <name>` (unit) or `--rule <id>` (rule) and reports each applicable gate (unit: validate / verify / appendix; rule: validate only) plus a `READY FOR PROMOTE` conclusion. A stable-only target (no candidate file) reports its confirmation states and drift state instead. |
| `fresh@candidate` | Read-only report of every active candidate's cache freshness. Runs `specflowctl fresh --scope candidate` and reports each unit and rule with a candidate file, sorted and grouped, with the overall `READY FOR PROMOTE: N of M` count. |
| `fresh@stable` | Read-only report of every stable unit and rule. Runs `specflowctl fresh --scope stable` and reports each target's two confirmation states — `validate` (dependencies/rules) and `verify` (code alignment + quality) — plus the baseline drift state (`OK` / `CHANGED` / `MISSING`, see Stable Drift Baseline in `framework/validation_cache.md`). Confirmation states use the gate vocabulary (`FRESH` / `STALE` / `MISSING`). |
| `fresh@all` | Read-only report of both active candidates and stable targets. Runs `specflowctl fresh --scope all`; `READY FOR PROMOTE` covers the candidate section only. |

`fresh` has no `full`/`targeted` distinction and no `:keyword` variant — it does not execute any check, it only inspects cache files, baseline files, and re-chunks files with the same dependency logic promote uses. It never writes, deletes, or touches caches or baselines, and it never runs validate/verify. A `fresh@` query is always safe to run and never invalidates a gate.

### Dependency Analysis (read-only)

| User says | What agent does |
|-----------|-----------------|
| `deps@all` | Runs `specflowctl deps` (scope `all` — every current-layer unit, candidate preferred, stable fallback). Reports the dependency graph (unit nodes + directed `unit_refs` edges), any cycle member lists, and the promotion order (dependencies first). Pure mechanical computation — no inference, no judgment, no file writes. |
| `deps@{unit}` | Runs `specflowctl deps --unit <name>` and reports that unit's dependency view: its `unit_refs` (depends on), its `rule_refs` (bound rules), the units that reference it, and whether it sits on a cycle. |
| `deps@{rule}` | Runs `specflowctl deps --rule <id>` and reports the units bound to the rule. For a bound rule (`b_rule_*`): the units that list it in their `rule_refs`; empty output means no consumers. For a global rule (`g_rule_*`): every current-layer unit — global rules apply to all units by default and are not repeated in `rule_refs`. A global rule with no rule file (removed, mistyped) is reported as not found. |

`deps` has no `full`/`targeted` distinction and no `:keyword` variant — it is a read-only structural report. It only reads spec files; it never writes, deletes, or touches caches or baselines, and it never runs validate/verify. It complements `next` (single-unit discovery) and `fresh` (gate freshness) without overlap: when `validate` FAILs a unit on a cycle, `deps@all` is the diagnosis step — the cycle members and the full edge set tell the user what to unbind before re-running validate.

Gate status vocabulary: `FRESH` (cache exists and satisfies the gate), `STALE` (cache exists but files changed, coverage is incomplete, or mode/result is invalid — re-running the gate fixes it), `MISSING` (no cache file — never run), `BLOCKED` (a failure record: validate/verify cache declares P0/P1 findings), `OK` (appendix gate: all appendices are covered by the validate cache).

Deletion has no gate requirement; see `framework/removal_workflow.md`.

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

**Validate keyword dictionary:** validate's targeted checks are the checks of `framework/unit_validate_checklist.md` (unit) or `framework/rule_validate_checklist.md` (rule). The common keywords: `structure` (Check 1), `design` (Check 2), `scope` (Check 3), `evidence` (Check 4), `acceptance`/`coverage` (Check 5), `affects` (Check 6), `cross-unit` (Check 7), `constraint` (Check 8), `sharing`/`surface` (Check 9, unit), `clarity` (Check 10, unit). A keyword that does not match any check name is a no-match — ask the user.

A keyword for verify may also match a file name; the agent maps it to the spec content that references that file.

## Shared Judgment Protocol

`framework/shared_judgments.md` defines public reuse, unit design judgments, stable requirement protection and schema 4 verify records. Public observations cannot be suppressed by a private rationale. Failed preserve tasks cannot be cleared or suppressed by synthesis; a peer-owned record-drift mismatch routes to the protected unit instead of blocking the current unit (see §Deferred findings). Accepted public tasks are reused or awaited rather than independently allocated to several runs.

## Coverage Model

A gate run fixes an immutable input snapshot and computes a **coverage set**: the judgment keys that must each receive exactly one verdict. The coverage key is the unit of judgment and of mechanical coverage checking. The planner fixes the set; the agent partitions it into reviewer **sessions** — the planner no longer assigns keys to a fixed plan.

1. **Deterministic coverage.** `gate-plan` fixes the run's input snapshot and computes the coverage set from the gate, target, layer, and mode — the same inputs always produce the same coverage set (see §Coverage keys). Every coverage key is non-empty and unique within the run.
2. **Agent-chosen batching, lens purity.** The agent groups coverage keys into sessions. A session's keys must have the same kind and come from one lens only (`alignment` or `quality`), and the verify `alignment` and `quality` keys never mix. Any independent read-only executor can complete a session — one agent, several parallel workers, a human, or CI — provided it is an independent context that does not hold the spec's writing context (see §Guarantee Boundary).
3. **Mechanical tracking.** Session state lives in the run state under `meta/gate_runs/<run_id>/sessions/` (see `framework/validation_cache.md` §Write Rules → Tooled writes). `specflowctl gate-submit` validates each report mechanically, parses it once into an immutable session result, and records the report text, parsed verdicts/findings/scopes, and digest together. A rejected session can be re-submitted; every attempt is kept. An accepted session is terminal. Every mutating gate command enters the same repository-local operating-system lock before it loads mutable run state and holds that lock until the whole transition is committed, so plan replacement, terminal session acceptance, and finalize publication are linearizable across independent processes while session execution itself stays parallel.
4. **Coverage closure.** `gate-finalize` refuses a run whose coverage set is not covered by exactly one accepted session per key (plus carried baseline judgments for delta/repair). This mechanical closure replaces the anti-skip guarantee the former fixed plan provided: a required key cannot be silently dropped.
5. **Resume from disk.** Run progress and parsed results are readable from run state alone: `specflowctl gate-status` reports coverage progress, session states, attempts, and the next action, so an interrupted run resumes without conversation context. Run identity is path-bound: the requested run id, containing directory, and embedded `run_id` must match the tooling-generated id form exactly, and every state path must remain under `meta/gate_runs/`; malformed identity fails before any write or cleanup.
6. **Mechanical completion.** `gate-finalize --run <id>` accepts no judgment flags. It derives the gate result, blocking state, severity counts, and per-check statuses from the accepted final synthesis (or the accepted sessions of a clean run), then writes the cache only when the input snapshot is unchanged and the assembled evidence covers the run's coverage set.

**Verify evidence discovery before planning.** For full, delta, and repair verify runs, the coordinator first uses `specflowctl next --unit <name>` to locate the selected spec and declared implementation paths. It then searches repository-content files for related tests, direct callers and callees, and context dependencies for every acceptance item, including items that a delta/repair run might carry. This pass collects repo-relative file paths only; it makes no alignment or severity judgment. List every discovered file in the `gate-plan` input manifest (`--inputs-file`, one entry per line), including files already in the declared code surface; the latter are already readable, but the explicit entries let delta/repair compare the discovered evidence with the baseline. The independent reviewers make the judgments after the plan fixes the file hashes. A newly discovered file absent from a delta/repair baseline's recorded evidence makes the plan cover the full verify scope, so no old judgment is carried against evidence it never considered.

If a reviewer discovers that a required file was missed, it returns `Verification could not complete — missing read ref: <repo-relative path>` without a verdict. The coordinator does not submit that text as a session report. It locates the missing file, re-runs `gate-plan` with the union of previously discovered and new paths, and executes the replacement run's coverage set again. A file that cannot be located must be reported as an incomplete verification, not treated as absent evidence for a PASS. `gate-plan` replaces the earlier open run; its accepted session reports are not carried into the replacement run.

### Session record

| Field | Meaning |
|-------|---------|
| `session_id` | Stable identifier derived from the assigned key batch (single key = the key; a batch = a deterministic `batch-<hash>`); unique within the run |
| `keys` | The assigned coverage keys (all from one lens for verify; one report kind for validate) |
| `kind` | The report shape the keys map to: `checks` (validate group, including the clarity check), `item` (verify alignment), `file` (verify quality) |
| `check_keys` | The report check keys the session's verdict lines must cover (validate: `check-{n}`; verify alignment: the item id; verify quality: the declared code file path) |
| `read_refs` | The exact snapshot entries this session may declare. `gate-submit` validates against this session-local set, not the run-wide snapshot |
| `status` | `pending` → `accepted` or `rejected` |
| `attempts` | One entry per submission: attempt number, timestamp, `result_digest`, status, rejection reason |
| `report` | The accepted report, stored verbatim for presentation/audit |
| `result` | The immutable parsed verdicts, findings, dependency scopes, and report digest used by the final synthesis and finalization |
| `consumed_result_digests` | For result-consuming sessions, the exact dependency result digests bound at acceptance |

### Coverage keys

| Gate | Lens | Coverage keys |
|------|------|---------------|
| `validate@unit` | — | the validate check groups, grouped by read surface: `structural` (checks 1, 3, 6), `design` (2, 4), `acceptance` (5), `dependencies` (7, 8, 9), `clarity` (Check 10) |
| `validate@rule` | — | the current check keys from `framework/rule_validate_checklist.md`, fixed by the plan |
| `verify@unit` | `alignment` | `item:<unit>:<item>` and related stable `preserve:<unit>:<item>` requirements |
| `verify@unit` | `quality` | `code:<file>`, `design:<unit>:<file>` and one `architecture:<unit>` |

The run's coverage set is the union of the applicable keys, each tagged with its lens. The planner computes it mechanically from the spec and declared surface; it does not assign keys to sessions.

Delta re-run set: the keys whose declared deps went stale ∪ the current keys the baseline never declared ∪ explicit `--rerun` keys; repair additionally includes the failure record's `fail` keys and persisted `invalidated_checks`. A key the baseline never declared — a new acceptance item, a code file added since the baseline — has no evidence to carry, so it must execute exactly like a stale judgment. When the re-run set covers every declared key, nothing is carried over: the plan is the full coverage set for the declaration and is reported as a full-scope re-run.

Stable-only targets use the same rules with `target: stable`. Rule targets have no verify.

**Verify planning requires at least one acceptance item.** A verify run's alignment work set is the acceptance items, so `gate-plan` locates them structurally and rejects a spec with an empty item set before any run state is written — a verify run with no alignment keys could finalize a pass cache with no spec-to-code evidence. The mechanical `specflowctl validate` Check 2 rejects the same spec.

### Input roles

The derived target surface and the input manifest have different roles:

- spec-derived `implementation_surface` / `affects.files` entries define the verify `alignment` evidence and, expanded to their files, the verify `quality` coverage keys;
- the input manifest (`--inputs-file`) adds evidence entries that every session may read and declare, but it never creates a coverage key;
- file, directory, and logical-reference forms of a manifest entry have the same semantics; directories expand into their repository-content evidence files only. Every physical path, including spec-derived paths, must resolve inside the project root; absolute external paths, lexical `..` escapes, and in-project symlinks that resolve outside the project are rejected before run state is written.

The manifest is scratch input, not evidence. It is a plain text file with one file, directory, or logical-reference entry per line; blank lines are skipped, and entries are used verbatim. Keep it under the project root's ignored local-state directory `meta/plan_inputs/` (the same tree as `meta/gate_runs/`; one file per plan, e.g. `verify-{unit}-candidate.txt`) or outside the repository: a manifest that is a repository-content file is rejected before the plan is fixed, so a scratch file can never enter the snapshot, the evidence corpus, or a delta comparison. `specflowctl clean` clears the directory on demand; no plan ever reads a manifest implicitly.

**Verify planning rejects an unresolvable code surface.** Before a verify run's input snapshot is fixed, `gate-plan` validates every acceptance item's `implementation_surface`: the exact `<pending>` placeholder is skipped (design-first), and every other value must be a single repository-relative file or directory path that resolves to at least one real file (a directory expands to its repository-content files — the files Git tracks plus untracked files that are not ignored; ignored dependencies and build output are not part of the surface). A semicolon list, wildcard pattern, nonexistent path, or a directory with no repository-content files rejects the plan with the item id, the value, and the reason — the declared surface can never silently expand to zero files. The mechanical `specflowctl validate` Check 3 applies the same rule.

This separation is part of the interface. A report declaration that belongs to the run snapshot but not to the submitting session's `read_refs` is rejected.

**Delta and repair scope are mechanism-derived.** `gate-plan --mode delta` reads the stale cache's per-check evidence: the re-run set is the keys whose declared deps went stale, the current keys the baseline never declared (a new acceptance item or code file has no evidence to carry, so it must execute), and explicit `--rerun` keys. A stale dependency no check declared is mapped by fixed association where one exists (on a unit validate target: a `unit:` entry → Check 7; a `rule:` entry → Check 8; on a rule target: a `unit:` entry → the optional-owner existence Check 4 — see §Incremental scope); where no fixed association exists or the cache carries no per-check evidence, the plan degrades conservatively to the full coverage set and reports the degradation. `gate-plan --mode repair` additionally reads the failed keys from the failure record's `status` map and the targeted contradictions from its machine-owned `invalidated_checks`. When the re-run covers every declared key, nothing is carried over and the plan is reported as a full-scope re-run ("the re-run covers every declared key — the plan covers the full scope"). The executor does not select the scope; it executes the planned coverage set.

### Session report contract

Each session report is the key-batch-scoped fragment of the gate's report. `gate-submit` parses it once into the session result:

- every assigned check key appears exactly once with a verdict token from the gate's allowed set (validate: `PASS | WARNING | FAIL`; verify `alignment`: `ALIGNED | MISMATCH | CANNOT_DETERMINE`; verify `quality`: public `FACTS`, a design conclusion with `spec_requirements`, or an architecture conclusion with six assessments); every validate/verify verdict carries a non-empty reason, and a `quality` `unacceptable` conclusion carries a non-empty reason — the non-blocking design conclusions take their basis from `spec_requirements`; architecture conclusions take it from the six Dimension 8 assessments;
- gate-specific session structure is complete: unit validate Check 5 reports sub-checks `5a` through `5i`, and the other validate groups (including Check 10's clarity group) report their checks in the standard checks format; a verify `alignment` session reports, per item, an `evidence` line, a `deterministic` line, `Part A`, `Part B`, and — for a MISMATCH — the inline finding fields; quality sessions follow `framework/shared_judgments.md`: one public or design block per file, or one architecture block per unit. Public facts preserve potential observations; design actively checks spec requirements and disposes observations; architecture assesses the six Dimension 8 fields once. Design and architecture conclusions, `gate_findings` and their P0/P1 findings must agree;
- every check key declares at least one `Dependency scope:` line whose file resolves inside that session's `read_refs` and whose declaration parses;
- findings receive stable run-scoped ids `{run_id}/{session_id}/F{n}` in report order (the run id is printed in the mission's `Run:` line);
- each finding is bound only to its actual report keys: alignment findings belong to their item, quality findings to their `File:` block, and a validate session judging multiple report keys declares `Finding affects: {finding_id} = {check_key}[, {check_key}...]` for each finding. A single-check validate finding belongs to that check. Every validate FAIL key must have its own P0/P1 finding, and a PASS or WARNING key must not have one. Sharing a session never adds other failed keys to a finding;
- findings authored in a session report carry the shared finding block from the unified report skeleton (`problem:` / `evidence:` / `impact:` / `fix:` or `decision:` — with gate-specific extra lines; `quality` P3 findings use their `fact_anchor:` line as the evidence form). The block's detail lines are contiguous indented lines directly under the finding's entry line; `gate-submit` stores them verbatim and `gate-finalize` re-renders them when a finding is carried into a delta/repair run;
- a verify `alignment` MISMATCH finding identifies its item and carries the finding block (`Problem:`, `Evidence:` with at least one spec-side and one code-side sub-line (each present side quoted verbatim; when a side is absent, its sub-line states the absence and the searched scope), `Impact:`, and exactly one of `Fix:` / `Decision:` matching the suggested direction, with at least one `Options:` entry for `Decision:`), plus root cause, suggested direction, severity, confidence, and dependency scope.

The main agent presents the unified report generated from the accepted artifacts. `gate-finalize` derives the run-level header (`Result`, `Blocking promote`, `Key counts`) and cache body; the main agent does not supply or recompute those values.

**Verify alignment report fields:** exactly one `Item: {id}`, `Problem: ...`, `Evidence:` followed by at least one spec-side and one code-side sub-line, `Impact: ...`, exactly one of `Fix: ...` (spec_gap / code_gap) or `Decision: ...` (needs_design / blocked) with at least one `Options:` entry, `Root cause: ...`, `Suggested direction: ...`, `Severity: P0|P1|P2|P3`, and `Confidence: high|medium|low`, followed by the item's dependency-scope lines. Root cause and direction values are the Step 7 vocabulary in `framework/unit_verify_checklist.md`. `gate-submit` renders the finding block into the finding's detail lines: the entry line plus `problem:` / `evidence:` / `impact:` / `fix:` or `decision:` / `options:` / `root_cause:` / `direction:` / `confidence:`.

### Final synthesis

A unit run executes one final session when it has assigned relationships to check or findings to dispose. Local PASS results do not remove assigned relationship work. A run with neither condition finalizes directly; rules have no final synthesis. Generate it with `specflowctl gate-mission --run <id> --final --format prompt` and submit it with `specflowctl gate-submit --run <id> --session cross --keys cross --report PATH`.

**Relationship scope at planning.** Before every unit full, delta, or repair plan, the coordinator compares the current change with the last applicable reviewed state and declares the relationships it touches through required `--relationships name[,name...]`, or `--relationships none`. This is a semantic judgment, not a count of changed files or local findings. Select the names from the relationship tables below. For a new target or an unusable baseline, assess the relationships present in the complete current design; do not declare `none` simply because no local defect is known. A declaration selects checks; it supplies no verdict. The tooling fixes the selection in run state and rejects names outside the gate's relationship vocabulary.

**Incremental relationship work.** Each relationship declares its own evidence with `relationship:<name>: <read_ref>: <declaration>` and publishes `Effective status: relationship:<name> = pass|fail`. Cache judgment schema 4 for unit verify, and schema 3 for unchanged cache roles, records the checked names and statuses. Full runs preserve unchanged relationship judgments from a usable baseline and recheck stale or failed ones. Delta/repair automatically union stale, failed (repair), invalidated, and explicitly selected relationships into the recheck set; `none` cannot exclude required work. Unchanged relationships are carried with their exact evidence. A run may have no local coverage keys and still require a relationship-only final session. New or changed relationships absent from the baseline must be selected by the coordinator; dependency tracking cannot discover an undeclared semantic connection.

**Execution scope.** Check only the assigned relationships between the current source parts and the accepted or carried judgments. Cite the relevant sides in the reason/evidence and declare every source region the comparison used. Do not repeat local alignment, quality, or validate checks. When the relationship list is empty, the final session only disposes existing findings and derives effective statuses. `cross:` scope lines declare evidence used for disposition or ownership; they do not replace per-relationship evidence. A new relationship finding must link to a failed assigned relationship and affect its `relationship:<name>` key, plus any local keys actually implicated. A new canonical merge finding may consolidate existing findings without introducing a new local audit.

The final synthesis does not repeat every local check. It checks relationships between the sessions' conclusions and the shared source material. Its result must:

1. bind the digest of every consumed current result and carried judgment;
2. dispose every input finding as retained, suppressed, or merged, with evidence for suppression/merge; every merge chain must terminate at one retained input or new finding, which is counted once with the merged findings' logical keys;
3. publish the effective status of every logical check/item/file/relationship plus `cross`;
4. publish any new synthesis finding with severity, evidence, and affected check keys, and — for validate/verify — link every failed assigned relationship to a new retained finding;
5. cover the complete logical result set, including carried judgments in delta/repair runs.

For verify, also publish exactly one final quality conclusion for every `design:<unit>:<file>` and `architecture:<unit>` key, including carried keys. This assessment consumes the accepted reports and the final finding dispositions; it does not repeat the local review. `unacceptable` is required exactly when a gate-driving canonical P0/P1 finding affects that key. Otherwise the reviewer chooses `acceptable` or `needs_attention` with a reason. P2/P3 findings and a logical `fail` do not imply `unacceptable`, and a logical `pass` does not erase `needs_attention`. A run without final synthesis preserves each accepted quality conclusion verbatim.

**Decision evidence persists with the affected judgments.** `cross:` declarations are dependencies of changed final quality conclusions and of the current logical keys affected by the findings being disposed or owned, even when suppression removes a finding from the final result. A terminal merge group's keys inherit the source judgments' evidence as well. `gate-finalize` merges this evidence into the affected per-check cache entries and their file-level dependency union, including keys carried from the baseline. Adding synthesis evidence to a carried judgment does not re-execute it. `cross` remains a summary status with no independent cache check entry. A change to this evidence stales the cache, selects the affected judgments for delta work, and prevents promote from consuming the old decision.

An omitted input result, finding, or logical status rejects the final synthesis. A failed assigned relationship without a new retained finding also rejects it; validate findings must be P0/P1, while verify may retain non-blocking P2/P3 relationship findings. The coordinator cannot replace or override an accepted final synthesis.

**Final synthesis lines:** in addition to its verdict and dependency scopes, the final synthesis report carries:

```text
Finding disposition: {input_finding_id} = retained
Finding disposition: {input_finding_id} = suppressed — {evidence-backed reason}
Finding disposition: {input_finding_id} = merged -> {retained_finding_id} — {reason}
Effective status: {logical_check_key} = pass | fail
Effective status: cross = pass | fail
# Verify only, exactly once for every design and architecture key:
Quality conclusion: {quality_key} = acceptable | needs_attention | unacceptable — {basis}
[P0|P1|P2|P3] {location} — {new synthesis finding}
Finding affects: {synthesis_finding_id} = {logical_key}[, {logical_key}...]
# Validate/verify only, exactly once for every failed assigned relationship:
Cross item finding: {failed_item_key} = {new_retained_synthesis_finding_id}
# Verify findings may be deferred to another unit by recorded ownership —
# quality ownership, or a protected unit's own stable-record drift:
Finding ownership: {retained_finding_id} = owned_by {unit} — evidence: {read_ref}; reason: {one line}
```

There is exactly one disposition per input finding and one effective status per logical check/item/file/relationship plus `cross`. Bracketed findings in the final synthesis are new synthesis findings; each carries the shared finding block on the indented lines directly under its entry line and must name at least one affected non-cross logical key. Retained local findings are referenced by disposition instead of copied. A `merged` disposition identifies a duplicate, not a removal: following its target repeatedly must reach a finding that is retained in the final result. A target that is suppressed, missing, self-referential, or part of a merge cycle rejects the synthesis. The terminal retained finding is counted once, and its final affected-key set is the union of its own keys and the source keys of every finding merged into it.

Each retained finding's severity is the severity assigned by the session that raised it. On retain or merge the final synthesis may raise a finding's canonical severity conservatively when the read evidence proves a larger impact — it never lowers it (see `framework/severity_policy.md` §9). A suppressed or non-terminal merged finding has no canonical severity. Finding ids are already visible in dependency results; a finding created by the report being authored uses the deterministic run-scoped id `{run_id}/{session_id}/F{n}` from its finding order (for example `20260916-101112-abc123/cross/F1`). Run-scoped ids are unique by construction across runs.

For every non-cross logical key, effective status is closed mechanically over the **gate-driving** retained-finding set: it is `fail` if and only if at least one gate-driving retained P0, P1, P2, or P3 finding affects the key, and is `pass` otherwise. A gate-driving finding is one owned by this run's unit or unassigned; a deferred finding (owned by another unit) is retained for audit and routed, but does not mark its keys and does not block this run. `Effective status: cross` must exactly mirror the synthesis verdict. `gate-submit` binds the dependency digests automatically from run state, so the report does not transcribe hashes. `gate-finalize` reapplies the accepted ownership records and canonical severities to the resolved terminal finding set and derives counts, blocking, result, and the carried judgment baseline only from the gate-driving canonical findings.

### Deferred findings

A unit's verify `quality` surface is the code its spec declares, and one physical file can carry the behavior of several units. A `quality` finding can therefore describe behavior whose **recorded ownership** belongs to another unit — the reviewed unit's own spec records a dependency-direction or boundary declaration (e.g. "child-run semantics are defined by the agent unit"), or another unit's spec records the behavior (reachable as ownership evidence when the coordinator listed it in the `gate-plan` input manifest). Such a finding must not block a unit that cannot legitimately repair it, and it must not be dropped.

The same dimension covers the reverse case: a protected stable requirement whose declared implementation mapping no longer resolves while its behavior is still implemented is the protected unit's own record debt — only that unit's promote can refresh the mapping — so its finding routes to that unit instead of blocking the reviewing unit. This is a **peer-owned record drift**. A protected requirement whose behavior is not implemented anywhere is not record drift: it stays a gate-driving mismatch and blocks the reviewing unit. The ownership dimension resolves both routings:

- **Ownership records are evidence-backed.** A deferrable finding is either a `quality` finding — every logical key it affects belongs to the quality lens, including carried judgments and merged source keys — or a **protected stable-record drift** — every affected key is a `preserve:{unit}:{item}` key of one single protected unit, and the record's owner is that unit. Nothing else is deferrable: an item finding, a merge containing one, a finding mixing quality and preserve keys, or a preserve finding naming several protected units stays unassigned and drives this unit's gate. The final synthesis writes one `Finding ownership: {finding_id} = owned_by {unit}` record per deferred finding. The record names a terminal retained finding, cites a read ref inside the final synthesis's `read_refs` covered by a `cross` dependency-scope declaration, and states its basis in its reason: the recorded ownership for a quality finding; evidence that the protected declaration no longer resolves while the behavior is implemented at its current location for a record drift. The tool validates the record's mechanics; whether the cited document really records that ownership is the executor's judgment. A finding without a record is unassigned and drives this unit's gate — deferral without recorded evidence is not available, so a real problem cannot be routed away by assumption.
- **Deferral routes, it does not delete.** A deferred finding stays in the report (`framework/unit_verify_checklist.md` §Output Format → Deferred findings) and in the cache's `GATE_JUDGMENTS` block as `deferred_findings`, but is excluded from the severity counts, from the blocking decision, and from the per-key effective-status closure. The reviewed unit's cache can pass while the finding is pending elsewhere. A routed protection finding's published decision for the protected key is the run's effective pass state — the stale mapping is the owner's debt, not this unit's failure.
- **The deferral is handed to the owner's next verify.** `gate-finalize` writes each deferred finding to the repository's deferred-findings ledger (`framework/validation_cache.md` §Format → Deferred-findings ledger), keyed by owner unit. The owner's next verify plan (`gate-plan --gate verify --unit {owner}`) loads the owner's pending entries into the run, rebinding source-unit `design:<unit>:<file>` and `architecture:<unit>` keys to the owner, and a protected record-drift key `preserve:{owner}:{item}` to the owner's own `item:{owner}:{item}` — the logical key owning that requirement's judgment in the owner's run — in both `source_key` and `affected_keys`, while preserving the finding id, source unit and source run. Public code and relationship keys keep their existing identities. Pending findings appear in the plan output and in the `gate-mission` context (the final synthesis always, the matching `quality` sessions as well). The owner's final synthesis must dispose every pending deferral exactly like an input finding — `retained` makes it the owner's own finding (and it blocks the owner if P0/P1), `suppressed` with a reason drops it, `merged` folds it into a retained finding, and a new ownership record re-routes a quality deferral to a third unit (a record-drift deferral affects the owner's own item key, so it cannot be re-routed). For a record-drift deferral, `retained` or `merged` is the correct disposal when the owner's current round does not reconcile the record, while `suppressed` with evidence records that the round reconciles the stale mapping and its promote refreshes it. The entry is consumed when the owner's run finalizes. A pending deferral whose key is not a logical key of the owner's run — a file outside the owner's verify surface, or an item absent from the owner's current spec — cannot be retained or re-routed: the synthesis must suppress it with an evidence-backed reason, and the state signals a spec-surface inconsistency — the owner's spec should declare the behavior and its current implementation location.
- **Deferred findings are not carried.** A unit's judgment baseline carries only its gate-driving findings; deferred findings live in the ledger, not in the carry chain. A later delta/repair run of the reviewed unit neither re-disposes nor re-defers them. When a later verify of the reviewed unit re-reviews the deferred file, its fresh judgment supersedes its own older deferrals for that file, and re-deferral writes a new ledger entry.

Deferral never overrides spec-first: code with no recorded design remains a finding, and without recorded ownership evidence it stays unassigned and blocks. The dimension changes only the route — a unit's own debt stays with the unit; another unit's recorded debt goes to that unit's round.

Promote does not read the ledger: a pending deferral is verify-routing state, not a gate condition. It becomes a gate condition for the owner when the owner's next verify loads and disposes it; until then it stays visible in the plan output, the `gate-mission` context, and `fresh`.

### Execution topology

Coverage keys define what must be judged and how coverage is tracked. The declared execution policy is **one independent read-only reviewer session per agent-chosen batch**: the agent may batch same-shape / small keys into one session and split a key that needs its own judgment, but a session never mixes lenses, and the judge of every key is a fresh subagent, never the main agent. A session's mission may carry the accepted results of the sessions it consumes — that is the designed input for the final synthesis — but the sessions themselves remain separate. This keeps reviewer independence an execution property: the executor must not hold the context in which the spec was written. The policy is declared, not mechanically verified: no run or session field records worker or platform identity, and the tooling verifies content and state, never execution shape (see §Guarantee Boundary).

## Guarantee Boundary

The gate chain combines two property classes. Only the first is mechanically checkable by the runtime-neutral tooling:

**Core-mechanical guarantees** — verified from artifacts at gate-plan, gate-submit, gate-finalize, `fresh`, and `promote` time:

1. **Input snapshot consistency** — a cache is written only by a gate run whose input snapshot (fixed at `gate-plan`, before execution) was byte-unchanged at `gate-finalize`; every declared file or region resolves to the recorded content CIDs, compared byte-for-byte
2. **Coverage** — every coverage key is covered by exactly one accepted session (plus carried baseline judgments for delta/repair); the assembled cache covers every key's report keys (validate additionally requires every non-exempt appendix). The code surface cannot be empty by accident: verify planning rejects any non-`<pending>` `implementation_surface` that yields no file — a missing path, or a directory with no repository-content files (§Coverage Model → Input roles) — so a code-file set can be empty only for `<pending>` items — a state verify Step 6 judges as MISMATCH
3. **Output structure** — the report and cache carry the required fields per the gate's own checklist
4. **Result closure** — when relationships are assigned or findings exist, the final synthesis is bound to the accepted session-result digests; `gate-finalize` derives result, blocking, counts, and statuses without coordinator-supplied judgment; a deferred finding's ownership record cites evidence inside the final synthesis's read refs and a unit that exists, so a deferral can never route a finding to nowhere
5. **Cache closure** — the derived result is persisted and checked through the fresh → promote chain

**Runtime-dependent properties** — the required execution shape, not observable from repository artifacts:

1. the run executes in an independent context — the executor does not hold the context in which the spec was written
2. a real separate worker executes it — not the main agent itself
3. the executor is read-only — no file modification, no state-changing commands, no further sub-agent launch

Session, worker, and permission are runtime-internal concepts; runtime-neutral tooling cannot observe them. A marker written by the run itself (`session_id`, `worker_id`, `read_only: true`) is a self-report, and the writer is the party whose independence is in question — self-reported execution shape is not evidence and must not enter any cache or report. The independence requirement remains in force as an execution policy: it is what keeps a verdict from being self-approval, but it is a premise of judgment quality, not a mechanically proven premise of the gate. No report may claim "independence verified".

**Declared vs attested:** independent execution is **declared** — required by the framework rules and held by executor discipline; this is its status today. It would be **attested** only if a signature from a provider the core trusts covered the execution facts. No attestation provider exists today, and none is required for the gates to run. If stronger assurance is ever needed, the only admissible mechanism is a runtime-neutral, property-based interface — input snapshot, executor isolation, write capability, output digest, provider, signature — implemented by runtime or CI adapters; core verifies trusted provider signatures only. A platform-specific session field or unsigned metadata is never `attested`.

## Sub-agent Prompt Assembly

For full, delta, and repair runs, `specflowctl` generates the session mission. The main agent does not assemble a prompt from this document or from `framework/validation_cache.md`. It plans the run with `gate-plan --format json`, partitions the coverage set into batches with the same kind and lens, reads `gate-status --run {run_id} --format json`, and for each batch runs `gate-mission --run {run_id} --keys {k1,k2,...} --format prompt` and passes that output verbatim to one independent read-only reviewer. A rejected session's regenerated mission includes the latest `gate-submit` rejection reason so the independent reviewer can correct the complete report. It submits the returned text with `gate-submit`, repeats mission/execution over the remaining uncovered keys, and — when relationships are assigned or the primary pass produced findings — runs `gate-mission --run {run_id} --final --format prompt`, submits that with `gate-submit --session cross --keys cross`, and runs `gate-finalize` only when `gate-status` reports next action `finalize` (full coverage and, when relationships are assigned or findings exist, an accepted final synthesis). The coordinator never supplies gate verdicts or cache fields.

`gate-plan` and `gate-status` JSON expose the run identity, coverage progress, and next action. JSON and text use the same next-action decision: `execute` while coverage is incomplete, `synthesize` when a unit run has assigned relationships or findings and no accepted final synthesis, `finalize` when ready to publish, and `none` after completion or invalidation. Rule runs never synthesize; both mission generation and submission reject `cross` for a rule target. `gate-mission --format json` exposes the mission as structured data: run and target, the session's `check_keys`, `read_refs`, dependency judgment records and carried judgment records (session id, kind, status, digest, verdicts, findings, analysis, effective status — carried records publish their effective status in place of verdicts), pending deferred findings, semantic checklist location, read-only constraints, mission context, report contract, and submission command. A rejected session additionally has `last_rejection` with the latest rejection reason; a first-attempt session omits it. A mission carries exactly one session batch and corresponds to one independent reviewer session (see §Execution topology). `--format prompt` renders the same mission as text ready for the reviewer. The default mission format is `prompt`.

### Check / session scope

The session's `check_keys` are the only current judgments to execute. Delta and repair missions never ask a reviewer to repeat a carried judgment. `read_refs` are the session's evidence inputs; the semantic checklist path is a protocol reference, not an evidence declaration. The reviewer declares `Dependency scope:` only for executed keys and only for files in that session's `read_refs`. `gate-submit` rejects missing, malformed, or out-of-scope declarations.

The reviewer reads the named checklist for semantic judgment: unit/rule validate checks, verify `alignment` (Steps 1–7), or verify `quality`. The final synthesis additionally reads `framework/severity_policy.md` §9. The mission supplies execution instructions and the text report contract, including the required fields and conditional lines. Report-contract data and `gate-submit` share the same fixed-field definitions; dynamic synthesis consistency remains mechanically checked at submission. A generated template is a filling aid, not evidence or a second semantic protocol.

The main agent launches the reviewer in a genuinely separate read-only session, sends the generated prompt verbatim, and collects the report verbatim. The reviewer may read files, search by pattern, and run read-only git queries; it must not modify files, run state-changing commands, or launch sub-agents. Tooling checks the submitted artifact and its bound inputs, but cannot observe who executed the session (see §Guarantee Boundary). Reviewers never write cache or run-state files.

Targeted checks remain in the main agent session: they do not have a coverage run or a generated mission (§Targeted Runs).

### Relationship vocabulary

The final synthesis checks relationships between the sessions' conclusions and the shared source material. Each assigned relationship is stated exactly once; each failed result links to a new retained finding. The tables below define the available names, not a mandatory full checklist. The final session checks only the names selected by the plan, then disposes existing findings.

#### Verify synthesis items

Checks for consistency between the affected source parts (spec sections, producer/consumer code, or their shared definitions):

| Check | What it looks for |
|-------|------------------|
| Contract consistency | Do affected producers and consumers use the same behavior and endpoint contract? |
| Data definition drift | Do affected source parts agree on field names, types, units, and enum values? |
| State machine coherence | Do affected transition rules agree across the participating source parts? |
| Error code conflict | Do affected source parts agree on error codes and the conditions they represent? |
| Cross-reference integrity | Do affected references resolve to the claimed definitions or behaviors? |

**Output:** the assigned per-relationship results, a finding link for each failed result, finding dispositions, effective logical statuses, and any new synthesis findings. The summary `Cross-check:` line is derived from the five results.

Fixed report keys, in table order: `contract_consistency`, `data_definition_drift`, `state_machine_coherence`, `error_code_conflict`, `cross_reference_integrity`.

#### Validate synthesis items

After the local validate sessions resolve:

| Check | What it looks for |
|-------|------------------|
| Design × Constraints | Does the design (Check 2) respect the global constraints and bound rules (Check 8)? |
| Coverage × Scope | Does the acceptance coverage (Check 5) actually prove the declared scope (Check 3)? |
| Cross-unit cohesion | Do individual unit decisions (Check 7) align with the combined design intent? |

**Output:** the assigned per-relationship results, a finding link for each failed result, plus the same complete disposition/effective-status synthesis contract. A local failure may be suppressed only when the final synthesis demonstrates that it is a false positive against the complete source set; the suppression remains visible in the audit body but is excluded from retained finding counts.

Fixed report keys, in table order: `design_constraints`, `coverage_scope`, `cross_unit_cohesion`.

## Delta Runs

Delta runs (`revalidate@{target}` / `reverify@{unit}`) restore a stale cache with a partial re-run instead of a full one. They are complete-coverage runs: the judgments whose dependency evidence went stale are re-executed, the rest are carried over from the previous run, and the cache is written with `basis: delta`. Promote trusts a delta cache exactly like a full one — carried-over judgments have unchanged dependency evidence recorded in the cache, which is the same trust basis the gate already uses (see §Relationship with Promote).

The delta scope is **derived by mechanism at plan time**: the cache's per-check evidence (`checks`, see `framework/validation_cache.md` §Format) maps every check to the regions it declared; the stale regions are matched against that map to compute the affected check set, and `gate-plan --mode delta` turns that set into the run's coverage set. The executor does not select the scope — it reports the planned coverage set, executes it, and covers the rest by carry-over. The map covers per-check declarations only: entries without a `checks` mapping (logical references, contract files, whole-file declarations, declare-heavy extras) have no check association; their staleness is **unclaimed**, mapped to checks by the fixed associations below (§Incremental scope) — where no fixed association exists the plan degrades conservatively (never silently carried over).

### When delta applies

A delta run has two baseline forms — a **pass baseline** (the stale-cache recovery) and a **failure baseline** (the failure-record recovery, §Failure recovery below):

1. **Pass baseline** — a cache exists for the gate, with `mode: full` and `result: pass`, and at least one judgment must re-execute: a `files` entry whose declared dependency CID is no longer present, or a current key the baseline never declared. If nothing must re-execute, report "cache is fresh — no incremental re-run needed" and stop. A blocked cache (P0/P1 findings) is not this baseline — it is the failure baseline instead.
2. **Failure baseline** — a cache exists for the gate as a **failure record** (`result: fail` + `blocking: true`; for validate: written by a candidate full-run FAIL or a delta re-run's FAIL; for verify: written by a delta re-run's FAIL or a candidate full-run FAIL). The recovery re-runs the failed judgments, persisted targeted invalidations, newly affected judgments, current keys the baseline never declared, and explicit `--rerun` overrides — no stale source is required for failed or invalidated judgments.
3. A MISSING cache has no baseline to carry over from — run the full command instead.

**Coverage is a derived conclusion, not a rule:** the re-run set is the affected checks plus the current keys the baseline never declared plus persisted targeted invalidations and explicit `--rerun` overrides. When that set covers **every** declared check, nothing is carried over — the plan is the full coverage set for the declaration, reported as a full-scope re-run ("the re-run covers every declared check — the plan covers the full scope"). This is not a special case of own-spec staleness: editing one section of your own spec stales only the checks that declared that section, and the delta plan re-runs exactly those (plus any new keys). Where no scope can be derived at all — the cache carries no per-check evidence, an unclaimed dependency has no fixed association, an invalidated key cannot be mapped to the current judgment surface, or the cache is stale for a cause the declared per-check evidence cannot attribute (e.g. a file entry with no dependency chunks, or the main file missing from the files list) — the planner degrades conservatively to the full coverage set and reports the degradation. The same conclusion applies to a failure-record recovery.

### Incremental scope (mechanism-derived)

1. `gate-plan --mode delta` derives its scope from the cache's per-check `checks` mapping: the affected check keys. Stale dependencies that no check declared — logical-reference entries (`unit:{name}` / `unit:{name}:appendix:{file}` / `rule:{id}`), contract files, whole-file declarations, and declare-heavy extras in the file-level union — are **unclaimed**; the planner maps them by the command's fixed association:
   - unit `validate`: a `unit:` entry → Check 7 (cross-unit); a `rule:` entry → Check 8 (constraint alignment).
   - rule `validate`: a `unit:` entry → the optional-owner existence check (Check 4).
   - `verify`: an unclaimed code file or dependency entry has no fixed association — the plan degrades to the full coverage set and reports the degradation.
   - When the cache has no per-check evidence, the plan degrades to the full coverage set and reports it. There is no compatibility shim: a merged verify cache without per-check evidence is invalid — `fresh@` and `promote` fail it closed and require `verify@{unit}`.
2. The plan maps the re-run keys to the coverage keys that own them (see §Coverage keys) and records `carried_keys` for the declared checks that keep their previous evidence. Current keys the baseline never declared — a new acceptance item or code file added since the baseline — join the re-run set: they have no baseline evidence to carry, so they must execute exactly like a stale judgment. A targeted run that found P0/P1 records the contradicted key through `gate-invalidate`; `gate-plan --mode repair` reads the persisted `invalidated_checks` list and re-runs that key instead of carrying it over. `--rerun` remains an explicit additional override, not the persistence mechanism for targeted findings.
3. `gate-plan` reports the plan explicitly before execution begins: the re-run coverage keys, the carried-over checks with the statement that their dependency evidence is unchanged, and any degradation or full-scope coverage statement. The executor starts only after the plan is printed; the scope is not negotiated at execution time.

### Execution

- Execute only the planned coverage keys. Carried-over checks are not re-executed: `gate-plan` copies their structured judgments, complete renderable finding details, and evidence from the current judgment-schema baseline into the immutable run, and the final synthesis consumes those carried judgments alongside the new session results. A baseline without the current judgment schema requires a full run. When the run is finalized, every terminal retained finding that no accepted session report already contains — a carried judgment or a deferral routed in from another unit — is rendered into the new human-readable findings body exactly once; counts without their corresponding finding detail are invalid.
- Any gate-driving retained P0/P1 finding writes a **failure record**: validate/verify write the cache with `result: fail`, `blocking: true`, tool-derived severity counts, a findings body, `basis: delta` (`basis: repair` when a repair run fails again), and the synthesis-derived per-check `status` map (`fail`/`pass` for the re-run checks, `carried` for unchanged carried-over checks). Promote must not proceed.
- On PASS, `gate-finalize` writes the cache with `mode: full`, `basis: delta`, a fresh `timestamp`, and a complete `files` list: accepted session declarations replace the re-run checks' evidence (new `hash` + `deps` + per-check `checks` breakdown, computed by the tooling and tagged with the session's lens), carried-over checks keep their baseline entries (their CIDs are unchanged by construction — they were not stale sources), and their entry's declare-heavy file-level deps (the remainder no check owns) stay in the file-level union — a re-run check's superseded deps are never merged back in. The files list stays complete (including the main spec and every appendix) so the appendix and main-file promote checks keep passing.
- The report uses the standard skeleton with `mode: delta` and an `Incremental scope:` section (re-run coverage keys + carried-over declaration) and the finalize note "cache written with basis: delta".

### Failure recovery

A delta or repair run over a **failure record** (`result: fail` + `blocking: true`; validate/verify delta FAIL writes it, a repair FAIL updates it with `basis: repair`, and a candidate validate/verify full FAIL writes one too — validate/verify full-run records with `basis: full`) restores the pass cache incrementally after the findings are resolved. This is the counterpart of the stale-cache recovery — same command, same trust model, different baseline. The recovery is identical regardless of `basis` (see §Failure handling by gate role in `framework/validation_cache.md`).

1. **Scope derivation (plan time)** — `gate-plan --mode repair` reads the record's per-check `checks` entries:
   - **No usable status map on a failure baseline** — a record that declares no status at all, or a status for only some checks, cannot say which judgments failed, so **nothing may be carried over**: the plan is the full coverage set, reported as a degradation ("the failure record carries an incomplete per-check status map — the plan covers the full scope"). A status map is usable only when every check the record's structured judgment baseline (`GATE_JUDGMENTS`) holds has a status, the status values are exactly `pass`/`fail`/`carried`, and no `carried` appears in a full-run record (`basis: full` has no carried judgments). A key-set mismatch between the status map and the judgment baseline, an unknown status value, and `carried` in a full-run record are malformed state and take the same degradation path — the planner never guesses a missing or unknown judgment outcome. This is the safe path for legacy blocking caches (written before the failure-recovery design) and for any fail/blocking record whose status map is not complete — absent status means **pass** only on a pass baseline, never on a failure baseline.
   - **Complete status map present** — the re-run set = {judgments whose `status` is `fail` in the failure record} ∪ {keys in the machine-owned `invalidated_checks` list} ∪ {judgments whose declared deps went stale against the current content (the fix changed content)} ∪ {explicit `--rerun` overrides} ∪ {the current keys the baseline never declared}. `gate-invalidate` writes the persisted invalidation list immediately after a targeted P0/P1 contradicts a recorded `pass`/`carried` judgment. Judgments with `status` `pass`/`carried` are carried over only when their evidence is unchanged and their key is not invalidated. An invalidated key that cannot be mapped to the current judgment surface degrades the plan to the full coverage set. A full-run record's status map has only `pass`/`fail` values (every judgment was re-executed by the full run; `carried` appears only in records written by a delta or repair run). The newly stale sources map by the same rules as the stale-scope path; when the re-run covers every declared check, nothing is carried over (see §Coverage above).
2. **Execution** — execute the planned coverage set. Any P0/P1 finding updates the failure record (fresh evidence, findings body, status map — the record stays). A full pass makes `gate-finalize` write the cache with `mode: full`, `basis: repair`: new evidence for the re-run checks, the original evidence for the carried-over ones, fresh `timestamp`. The plan declares the recovery scope explicitly before execution, as for any delta run.
3. **No stale source required** — the failed judgments are the re-run reason; the "cache is fresh" stop does not apply to a failure baseline.
4. **Precondition** — the failure record must be present and its dependency evidence must still hold for the carried-over judgments (a stale record's carried-over evidence is untrusted: derive the scope from the stale sources per §Incremental scope and re-check conservatively — declare-heavy). A MISSING record is not a baseline: run the full command. A record whose per-check `status` map is absent or incomplete is not a failure-recovery baseline for the status-less judgments — step 1's no-usable-status-map path degrades it to a full re-run (fail-closed: carrying over unresolved P0/P1 findings is not allowed).

The failure record's `basis` records its writer: `delta` for a delta-FAIL record, `repair` for a repair-FAIL record (a recovery run that fails again keeps the record as the failure-recovery baseline), `full` for a full-run record (candidate validate/verify full FAIL, stable-only confirmation FAIL). The recovery's pass cache is marked `basis: repair` for audit separation. Promote trusts a `basis: repair` cache exactly like `full`/`delta` — the evidence rules are identical (see §Trust statement).

### Trust statement

A delta or repair cache carries no lower evidence standard than a full cache: every judgment in it is backed by dependency evidence present in the cache at promote time — re-run judgments by their new CIDs, carried-over judgments by their unchanged CIDs. The gate verifies the same mechanical checks for all three. `basis: delta` / `basis: repair` is audit metadata (see `framework/validation_cache.md` §Format); the promote gate does not distinguish full, delta, or repair.

### Layer applicability

Delta re-runs apply to **candidate targets** and to **stable-only targets with a usable baseline**. For a candidate target, a delta run restores promote eligibility, which exists only for the candidate layer. For a stable-only target, a delta run restores the stale confirmation state — it is a recovery, not a complete re-confirmation: it re-runs only the judgments whose dependency evidence went stale and carries the rest over from the pass baseline. The precondition is the same as for candidates — the gate's confirmation cache must exist with `mode: full` and `result: pass`. A MISSING cache (never confirmed) has no usable baseline — `gate-plan` reports: "No usable confirmation baseline. Run the full `{command}@{target}` (confirmation check) first, or `specflowctl fork {--unit|--rule} {name}` to start a new round." A blocked/failed confirmation cache is a failure record: the failure recovery applies to stable-only targets the same way (restoring the confirmation state with `basis: repair`); when the stable content itself can no longer hold against the changed dependency or rule, the record stays and forking reconciles it (see §Stable-only Targets → On FAIL).

The layer boundary is decided by file existence: a target with a candidate file is a candidate target; a target with only stable files is a stable-only target. A delta run that would apply against the candidate file is valid even when the target's stable counterpart exists — `fresh@` separates the two cache sets with layer-specific checks: the stable validate/verify variants require the stable main spec in the cache's files list (their caches must prove the main file was read).

## Stable-only Targets

When no candidate exists, applicable full commands run as **confirmation checks** of stable truth: validate and verify for a unit, validate for a rule. They write confirmation caches (`target: stable`) but never edit content. Normal design changes first fork stable to candidate (`framework/concepts.md`). Confirmation caches grant no promote eligibility and are consumed only by `fresh@stable`.

Stable confirmation caches have **two producers** with identical semantics:

1. **A `@stable` confirmation run** — an explicit user-triggered `validate@{target}` / `verify@{unit}` against a stable-only target, writing a fresh `target: stable` cache from that run's judgment.
2. **A successful promote** — `specflowctl promote` rewrites the candidate gate caches into `target: stable` confirmation caches (inverse of the fork rewrite): `target: candidate` → `target: stable`, and every physical path under `docs/specs/.../candidate/` → the `stable/` equivalent (see `framework/validation_cache.md` §Cache lifecycle). The promoted content is byte-identical to the candidate, so the caches' evidence is still valid for the promoted files. This gives every promoted target an immediate stable confirmation baseline and makes the stable-layer delta recovery (`re*`) usable without a full confirmation run first.

| Command | Relationship confirmed | Cache on PASS | On FAIL |
|---------|----------------------|---------------|---------|
| `validate@{unit}` / `validate@{rule}` | Stable content vs its dependencies and rules (Check 6/7/8, and Check 9 for units: referenced files, cross-unit contracts, global and bound rules, declared file associations) | `target: stable` validate cache | Write a failure record (`result: fail` + `blocking: true`); recommend forking the unit/rule to reconcile the stable content with the changed dependency or rule. The record keeps the confirmation state visible as BLOCKED and is the failure-recovery baseline |
| `verify@{unit}` | Code vs the stable spec (drift confirmation) | `target: stable` verify cache (VERIFIED state) | Write a failure record; report the drift and recommend forking (do not enter divergence resolution — see `framework/unit_verify_checklist.md` §Stable-only mode) |

Rule targets have no verify (rule verify has been removed), so the rule confirmation is `validate@` alone.

The confirmation caches go stale when their dependency evidence changes: the validate cache when a dependency unit's contract, a rule, or a referenced file changes; the verify cache when the code changes. Recovery from STALE: a delta re-run (`revalidate@{target}` / `reverify@{unit}`) re-runs only the judgments whose evidence went stale and rewrites the cache with `basis: delta` — the default recovery path when a pass baseline exists (see §Delta Runs → Layer applicability). Recovery from MISSING (never confirmed): the full confirmation run of the same command. A blocked/failed confirmation cache is a failure record: recover it with the failure-recovery delta run (`basis: repair`), or fork when the stable content itself no longer holds. The stale signal doubles as impact detection: after a rule change, every stable unit bound to the rule shows `validate: STALE` in `fresh@stable`, naming the impact surface.

## Targeted Runs

Targeted runs are available only through explicit user choice:

- `validate@{target}:check-{n}` — run a single specific check
- `validate@{target}:{keyword}` — run checks matching a keyword
- `verify@{unit}:{keyword}` — verify the spec content matching a keyword

When the user explicitly targets, the agent still reads the **full document** for context but only reports on the requested check(s)/content/file.

**Targeted runs never publish a complete result cache.** They are iterative feedback only, and never satisfy the promote gate. If a targeted run finds P0/P1, the coordinator immediately runs `specflowctl gate-invalidate` with the gate identity and affected check key. The command deletes a matching pass cache; for a failure record it preserves the record and persists the key in `invalidated_checks`, so a later repair plan re-runs it automatically. In the same locked transition it invalidates the contradicted immutable verify judgments and marks a matching open gate run invalidated, preventing consuming caches or earlier plans from using contradicted evidence. Acceptance-item ids resolve to the current decision for the requested layer's spec context, including when the unit cache is absent; a different candidate context leaves stable decisions intact. The targeted result itself is still not a complete gate result (see §Delta Runs → Failure recovery).

**Targeted execution shape:** targeted runs execute directly in the main agent session — no sub-agent is launched, no prompt is assembled per §Sub-agent Prompt Assembly, and no gate run or coverage set is generated. They are lightweight single-check feedback outside the promote gate, so the independent-session execution requirement does not apply to them (see §Guarantee Boundary).

## Output Format

Results are presented using the unified report skeleton defined in each gate's checklist (§Output Format in `framework/unit_validate_checklist.md`, `framework/unit_verify_checklist.md`, `framework/rule_validate_checklist.md`). The examples below show the skeleton in use.

### Full result (verify)

```
────────────────────────────────────────────
verify@user_auth · full · candidate
Result: PASS
Blocking promote: no
Key counts: Findings: 0 (P0: 0 | P1: 0 | P2: 0 | P3: 0)
────────────────────────────────────────────
Findings: none
────────────────────────────────────────────
Items:
  - AUTH-AC-001: ALIGNED — src/auth/login.go:42
  ...
Coverage:
  - items_with_deterministic_evidence: 10/10
  - items_reading_only: 0
Cross-check: 5/5 PASS
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
Findings: none
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
This was a targeted check — no complete cache was written.
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
Findings: none
────────────────────────────────────────────
1. Structural integrity: PASS
2. Design soundness: PASS
...
Cross-check: 3/3 PASS
Failed checks: 0 | Advisory findings: 0
────────────────────────────────────────────
Next step: None
────────────────────────────────────────────
Full validation passed.
```

### Full result with a finding (validate)

```
────────────────────────────────────────────
validate@user_auth · full · candidate
Result: FAIL
Blocking promote: yes
Key counts: Findings: 1 (P0: 0 | P1: 1 | P2: 0 | P3: 0)
────────────────────────────────────────────
Findings:
  Decision group (1 item) — need confirmation:
    1. [P1] AUTH-AC-003 — SessionErrorCode enumeration disagrees between the spec body and the acceptance item (needs_decision)
      problem: the body declares six SessionErrorCode values (including session_not_ready) while the item's pass_condition claims exactly five (without session_not_ready).
      evidence:
        - body: `type SessionErrorCode = "session_not_found" | ... | "session_not_ready";` — unit_user_auth.md, item AUTH-AC-003
        - item: `pass_condition: "SessionErrorCode is exactly session_not_found / ... / session_capacity (five values)"` — unit_user_auth.md, item AUTH-AC-003
      impact: the acceptance surface cannot cover session_not_ready; the implementation and the body semantics cannot both hold.
      decision: should the item be extended to six values, or should the recovery path stop returning session_not_ready?
      options:
        - extend the item's description and pass_condition to six values
        - remove session_not_ready from the recovery path
      ref: unit_user_auth.md, item AUTH-AC-003
────────────────────────────────────────────
1. Structural integrity: PASS
...
5b. Content alignment: FAIL — body/acceptance item conflict (finding AUTH-AC-003)
Failed checks: 1 | Advisory findings: 0
────────────────────────────────────────────
Dependency scope:
  check-5: docs/specs/units/candidate/unit_user_auth.md: acceptance_items
────────────────────────────────────────────
Next step: Resolve the findings, then re-run `validate@user_auth:check-5` to confirm
────────────────────────────────────────────
```

### Validate targeted

```
────────────────────────────────────────────
validate@user_auth · targeted (user requested: check-3 — scope integrity) · candidate
Result: PASS
Blocking promote: no
Key counts: Findings: 0 (P0: 0 | P1: 0 | P2: 0 | P3: 0)
────────────────────────────────────────────
Findings: none
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
This was a targeted check — no complete cache was written.
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
Findings: none
────────────────────────────────────────────
Incremental scope:
  - check-7 (cross-unit): re-run — dependency unit auth's acceptance item set changed (AC-003)
  - checks 1-6, 8-10: carried over — dependency evidence unchanged
Cross-check: 3/3 PASS
────────────────────────────────────────────
Dependency scope:
  check-7: docs/specs/units/candidate/unit_user_auth.md: all
  check-7: unit:auth: acceptance_item:AUTH-AC-003
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

When validate or verify produces findings, the agent presents
the findings to the user. P0/P1 findings (FAIL) stop the agent — it must not
proceed to promote and waits for a decision per HARD RULE 3a. verify P2/P3
findings (PASS with pending items) are non-blocking — the agent reports them and may continue (promote is
not stopped). No structured resolution menu is used.

File-specific format and details:

- `framework/unit_verify_checklist.md` §Step 7 — verify summary format and direction table
- `framework/unit_validate_checklist.md` §Present Findings — validate summary format

## Cache Interaction

### Write rules

| Event | Cache action |
|-------|-------------|
| `validate@{target}` full PASS | Write `validate_result.md` with `mode: full`, and `hash` + `deps` dependency evidence for all read files (entries are assembled at `gate-finalize` from the accepted session reports; see `framework/validation_cache.md` §Write Rules → Tooled writes) |
| `validate@{target}` full FAIL (candidate) | Write a failure record (`result: fail` + `blocking: true`, per-check `status` map — derived mechanically by `gate-finalize` from the accepted session verdicts) — the `revalidate@{target}` failure-recovery baseline. Promote must not proceed (see §Failure handling by gate role in `framework/validation_cache.md`) |
| `validate@{target}` full FAIL (stable-only) | Write a failure record (`result: fail` + `blocking: true`) and recommend forking (see §Stable-only Targets) |
| `verify@{unit}` full PASS (all aligned and every declared code file passing) | Write `verify_result.md` with `result: pass`, `mode: full`, `blocking: false`, `hash` + `deps` dependency evidence, and the merged `alignment` + `quality` sections |
| `verify@{unit}` full PASS (P2/P3 non-blocking findings) | Write `verify_result.md` with `result: pass`, `mode: full`, `blocking: false`, severity counts (`p0_count`...`p3_count`), `hash` + `deps` dependency evidence, and the merged `alignment` + `quality` sections. Promote may proceed |
| `verify` full FAIL (any P0/P1 findings, candidate) | Write a failure record (`result: fail` + `blocking: true`, per-key `status` map — derived mechanically by `gate-finalize` from the accepted session verdicts) — the `reverify@{unit}` failure-recovery baseline. Agent must stop, not proceed to promote. (Anchored to the validated spec, so `pass` keys with unchanged evidence may be carried over — see §Failure handling by gate role) |
| `verify` full FAIL (stable-only) | Write a failure record; report the drift and recommend forking |
| `revalidate@{target}` / `reverify@{unit}` delta PASS | Rewrite the gate's cache with `mode: full`, `basis: delta`: new evidence for re-run keys, original evidence for carried-over keys (see §Delta Runs) |
| Delta run FAIL (P0/P1 findings) | Write a failure record — rewrite the gate's cache with `result: fail`, `blocking: true`, severity counts, findings body, `basis: delta`, and the per-check `status` map. Promote must not proceed (see §Delta Runs → Failure recovery) |
| Any targeted run (`:check-{n}` / `:{keyword}`) | PASS with only P2/P3 findings → report findings and leave cache state unchanged. P0/P1 findings → run `gate-invalidate`: delete a pass cache or persist the key on a failure record, and invalidate a matching open run. Targeted runs never publish a complete cache result |

### Cache semantics

- A cache exists only when a complete-coverage run (full or delta) completed. `mode: full` is the only value ever written; `basis` records whether the cache came from a full run (`full` or absent), an incremental one from a pass baseline (`delta`), or an incremental recovery from a failure record (`repair`).
- A targeted PASS never writes or downgrades an existing cache. A targeted P0/P1 records blocking invalidation through `gate-invalidate`: a pass cache is deleted; a failure record is kept with the contradicted key persisted for repair.
- Any FAIL at full granularity writes a failure record for validate and verify (validate/verify records declare `pass`/`fail` for every judgment). Any FAIL at delta granularity writes a failure record for both gates. P0/P1 at any granularity means promote must not proceed.

## Relationship with Promote

- `specflowctl promote --unit <name>` requires fresh caches for validate, verify, and appendix coverage. The merged verify cache must cover **both lenses**: its `alignment` section and its `quality` section must each record a check for every expected coverage key. Every complete-result cache has `mode: full` by construction (targeted runs never publish one; delta runs keep `mode: full` and record `basis: delta`/`repair` — the promote gate checks the same evidence for all three).
- The validate cache must have `result: pass`; the verify cache must have `result: pass` (P2/P3 pending items are carried by the severity counts — non-blocking findings pass the promote gate).
- A failure record (`result: fail` + `blocking: true`) in any gate is rejected as BLOCKED — the findings must be resolved first. A `result: fail` cache without consistent blocking declarations is an invalid write and fails closed.
- No cache at all → promote rejected.

## State Transition Disclosure

| Cache state | Disclosure |
|-------------|-----------|
| validate cache fresh | "Validate passed all checks" |
| verify cache fresh (both lenses present) | "Verify passed all content and code quality" |
| verify cache fresh (result: pass, blocking: false, p2_count/p3_count > 0) | "Verify passed with P2/P3 pending findings — promote may proceed" |
| Failure record (blocking) | "Gate found P0/P1 findings — resolve them, then the delta re-run recovers incrementally" |
| No cache / stale | "Cache does not exist or is expired, needs re-checking" |

After a targeted run, the agent offers:
- "Run `verify@user_auth` for complete verification"
- "Or specify a section to verify by keyword"

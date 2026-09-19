# Verification Scope

## Problem

`validate`, `verify`, and `review` operate on a spec unit against the relevant codebase or document set. The older scoped/full dual-mode design used git working-directory changes to pick a subset. Practice showed that subset results are structurally unusable: the promote gate only accepts results from a complete run, so a scoped result always had to be re-run in full before promote — the scoped pass was wasted work. The subset also did not match what the user actually cares about: git diff reflects "what was recently changed", not "what the user is worried about".

The one reason for scoped mode was context saturation on large codebases. That problem is solved structurally by gate work packets (see §Gate Work Packets below) — a run decomposes into deterministic, independently completable packets instead of shrinking the checked surface.

## Solution

There are two complete-coverage run modes: **full** (`validate@{target}`, `verify@{unit}`, `review@{unit}`) and **delta** (`revalidate@{target}`, `reverify@{unit}`, `rereview@{unit}`). A full run checks everything in scope. A delta run re-checks the judgments whose dependency evidence went stale and carries the rest over — the result is a complete check either way (see §Delta Runs).

Targeted checking exists only through explicit user choice: `:check-{n}` and `:{keyword}`. The user declares the focus, not git diff. Targeted runs are for iterative feedback — they never write a cache, so they can never satisfy the promote gate.

**Cache invariant:** only a complete-coverage run (full or delta) writes a cache. A cache exists means a complete check passed (or, for review, a complete review completed). Targeted runs report findings and write nothing.

**Layer roles:** the verification loop — caches, promote eligibility, and delta re-runs — belongs to the candidate layer. The stable layer has no gate; the three full commands run against a stable-only target as **confirmation checks** of the stable content's continuing relationship with the outside world (see §Stable-only Targets). They write a `target: stable` cache consumed by `fresh@stable` and never edit anything — any change to consensus content goes through `fork` (see `framework/concepts.md` §4). Delta re-runs restore a stale confirmation cache when a usable pass baseline exists (see §Delta Runs → Layer applicability).

## Principles

1. **Full by default, complete always** — every command without a `:` suffix runs the complete check; delta runs (`re*`) also produce a complete check by re-checking the stale part and carrying the rest over. No git-awareness, no subset selection.
2. **Targeted only on explicit user choice** — `:check-{n}` (validate) and `:{keyword}` (all three commands) scope the run to a user-declared focus.
3. **Targeted results never satisfy promote** — targeted runs never publish a complete result cache. A targeted P0/P1 may only delete a pass cache or persist invalidation metadata through `gate-invalidate`. Only a complete-coverage run's cache passes the promote gate.
4. **Delta only on explicit user choice** — `revalidate@{target}` / `reverify@{unit}` / `rereview@{unit}` re-run only the judgments whose evidence went stale (plus cross-check — unit targets) and write a cache with `basis: delta`. The incremental scope is derived from the stale cache evidence and reported to the user explicitly (see §Delta Runs).
5. **Delta restores both layers** — the incremental re-run restores promote eligibility for candidates and a stale confirmation state for stable-only targets (whose confirmation cache exists with `result: pass`, review: `blocking: false`); a failure record (BLOCKED cache) is restored by the failure-recovery delta run (`basis: repair`). A delta run against a stable-only target without a usable baseline reports "No usable confirmation baseline. Run the full `{command}@{target}` (confirmation check) first, or `specflowctl fork {--unit|--rule} {name}` to start a new round." (see §Delta Runs → Layer applicability).
6. **Keyword means "what the user wants to look at"** — each command resolves the keyword inside its own target domain (see Keyword Resolution).
7. **No fixed item definition** — the framework does not define what an "item" is. The agent reads the spec structure dynamically.

## Syntax

### Validate

| User says | What agent does |
|-----------|-----------------|
| `validate@{target}` | Full: all 8 checks + cross-check (unit only — rules have no cross-check). Candidate target: writes the validate cache (`mode: full`, promote gate). Stable-only target (no candidate file): the same 8 checks against the stable content and its current dependencies and rules — writes the cache with `target: stable` (confirmation state consumed by `fresh@stable`); on FAIL writes a failure record and recommends forking (see §Stable-only Targets). Writes via the packet-run sequence (`specflowctl gate-plan` before the executor reads any input, `specflowctl gate-packet` before every packet executor with its output included verbatim in that executor's prompt, one `specflowctl gate-submit` per required packet report, `specflowctl gate-finalize` after every packet is resolved — `specflow/tooling/bin/specflowctl-<os>-<arch>`, `<tooling-root>` is `specflow/tooling`) — see §Gate Work Packets and `framework/validation_cache.md` §Write Rules (Tooled writes). Main agent MUST plan the gate run before the executor reads any input and finalize it only after every packet is resolved; otherwise `fresh` stays `MISSING` (or `BLOCKED` for a failure record) and `promote` is rejected — see `framework/unit_validate_checklist.md` §Completion — Persist Gate Cache. |
| `validate@{target}:check-{n}` | Targeted: single check `{n}` only. User explicitly chooses focus. Does not write a cache. Targeted runs intentionally do NOT write a cache; completion is the report alone (see `framework/unit_validate_checklist.md` §Completion — Persist Gate Cache). |
| `validate@{target}:{keyword}` | Targeted: matches keyword to a check name (e.g., "design" → Check 2, "scope" → Check 3). Does not write a cache. Targeted runs intentionally do NOT write a cache; completion is the report alone (see `framework/unit_validate_checklist.md` §Completion — Persist Gate Cache). |

### Verify

| User says | What agent does |
|-----------|-----------------|
| `verify@{unit}` | Full: verify all spec content (all 7 steps, a detection packet and conditional analysis packet per acceptance item) + cross-check. Candidate target: writes the verify cache (`mode: full`, promote gate); on FAIL (P0/P1) writes a failure record with the per-item `status` map — the `reverify@{unit}` failure-recovery baseline (see §Failure handling by gate role in `framework/validation_cache.md`). Stable-only target (no candidate file): verify the stable spec against the code (drift confirmation) — writes the cache with `target: stable` (VERIFIED state consumed by `fresh@stable`); on MISMATCH writes a failure record, reports the drift, and recommends forking (see §Stable-only Targets). Writes via the packet-run sequence (`specflowctl gate-plan` before the executor reads any input, `specflowctl gate-packet` before every packet executor with its output included verbatim in that executor's prompt, one `specflowctl gate-submit` per required packet report, `specflowctl gate-finalize` after every packet is resolved — `specflow/tooling/bin/specflowctl-<os>-<arch>`, `<tooling-root>` is `specflow/tooling`) — see §Gate Work Packets and `framework/validation_cache.md` §Write Rules (Tooled writes). Main agent MUST plan the gate run before the executor reads any input and finalize it only after every required packet is resolved; otherwise `fresh` stays `MISSING` (or `BLOCKED` for a failure record) and `promote` is rejected — see `framework/unit_verify_checklist.md` §Completion — Persist Gate Cache. |
| `verify@{unit}:{keyword}` | Targeted: matches keyword to spec content (section title, feature name, API path, etc.) → verify that content. Does not write a cache. Targeted runs intentionally do NOT write a cache; completion is the report alone (see `framework/unit_verify_checklist.md` §Completion — Persist Gate Cache). |

### Spec Review

| User says | What agent does |
|-----------|-----------------|
| `review@{unit}` | Full: read all files referenced in the candidate spec's `affects.files` and `implementation_surface` across all acceptance items → review those files with spec context. Candidate target: writes the review cache (promote gate). Stable-only target (no candidate file): review with the stable spec as design context (code-quality confirmation) — writes the cache with `target: stable`; implementation-class defects may be fixed in code and re-reviewed, design-class defects lead to forking (see §Stable-only Targets). Writes via the packet-run sequence (`specflowctl gate-plan` before the executor reads any input, `specflowctl gate-packet` before every packet executor with its output included verbatim in that executor's prompt, one `specflowctl gate-submit` per required packet report, `specflowctl gate-finalize` after every packet is resolved — `specflow/tooling/bin/specflowctl-<os>-<arch>`, `<tooling-root>` is `specflow/tooling`) — see §Gate Work Packets and `framework/validation_cache.md` §Write Rules (Tooled writes). Main agent MUST plan the gate run before the executor reads any input and finalize it only after every packet is resolved; otherwise `fresh` stays `MISSING` (or `BLOCKED` for a failure record) and `promote` is rejected — see `framework/spec_review_checklist.md` §Completion — Persist Gate Cache. |
| `review@{unit}:{keyword}` | Targeted: matches keyword to a file name in `affects.files` or `implementation_surface` → review that file. Does not write a cache. Targeted runs intentionally do NOT write a cache; completion is the report alone (see `framework/spec_review_checklist.md` §Completion — Persist Gate Cache). |

### Rule (validate only, verify removed)

| User says | What agent does |
|-----------|-----------------|
| `validate@{rule}` | Full: all 8 checks (7 metadata + 1 body quality). Writes the validate cache (`mode: full`, promote gate) via the packet-run sequence (`specflowctl gate-plan` before execution, `specflowctl gate-packet` before the packet executor with its output included verbatim in that executor's prompt, one `specflowctl gate-submit` for the single required report, `specflowctl gate-finalize` after that packet is accepted; `specflow/tooling/bin/specflowctl-<os>-<arch>`, `<tooling-root>` is `specflow/tooling`) — see §Gate Work Packets and `framework/validation_cache.md` §Write Rules. Main agent MUST plan the gate run before the executor reads any input and finalize it only after the packet is accepted; otherwise `fresh` stays `MISSING` (or `BLOCKED` for a failure record) and `promote` is rejected — see `framework/rule_validate_checklist.md` §Completion — Persist Gate Cache. |
| `validate@{rule}:check-{n}` | Targeted: single check `{n}` only. User explicitly chooses focus. Does not write a cache. Targeted runs intentionally do NOT write a cache; completion is the report alone (see `framework/rule_validate_checklist.md` §Completion — Persist Gate Cache). |
| `validate@{rule}:{keyword}` | Targeted: matches keyword to a check name. Does not write a cache. Targeted runs intentionally do NOT write a cache; completion is the report alone (see `framework/rule_validate_checklist.md` §Completion — Persist Gate Cache). |

> `verify` on a Rule target has been removed. If the user says `verify@{rule}`, report: "Rule verify has been removed. Run `validate@{rule}` instead." See `framework/concepts.md` for context.

### Delta re-runs (re* instruction family)

| User says | What agent does |
|-----------|-----------------|
| `revalidate@{target}` | Delta: re-run only the checks whose dependency evidence went stale + cross-check (unit targets), carry the rest over; a failure-record baseline re-runs the failed checks instead (see §Delta Runs → Failure recovery). Writes a cache with `mode: full`, `basis: delta` (`repair` from a failure record); delta FAIL writes a failure record. Writes via the packet-run sequence (`specflowctl gate-plan --mode delta` (or `--mode repair` from a failure record) before the re-run, `specflowctl gate-packet` before every packet executor with its output included verbatim in that executor's prompt, one `specflowctl gate-submit` per required packet report, `specflowctl gate-finalize` after every packet is resolved — `specflow/tooling/bin/specflowctl-<os>-<arch>`, `<tooling-root>` is `specflow/tooling`) — see §Gate Work Packets and `framework/validation_cache.md` §Write Rules (Tooled writes). Candidate targets, plus stable-only targets with a usable baseline (see §Delta Runs → Layer applicability). Preconditions and scope rules in §Delta Runs. Main agent MUST plan the re-run before it starts and finalize it only after every packet is resolved; otherwise `fresh` stays `MISSING`/`BLOCKED` and `promote` is rejected — see the target-appropriate checklist §Completion — Persist Gate Cache. |
| `reverify@{unit}` | Delta: re-verify only the spec content whose evidence went stale + cross-check, carry the rest over; a failure-record baseline re-runs the failed judgments instead. Writes a cache with `mode: full`, `basis: delta` (`repair` from a failure record); delta FAIL writes a failure record. Writes via the packet-run sequence (`specflowctl gate-plan --mode delta` (or `--mode repair` from a failure record) before the re-run, `specflowctl gate-packet` before every packet executor with its output included verbatim in that executor's prompt, one `specflowctl gate-submit` per required packet report, `specflowctl gate-finalize` after every packet is resolved — `specflow/tooling/bin/specflowctl-<os>-<arch>`, `<tooling-root>` is `specflow/tooling`) — see §Gate Work Packets and `framework/validation_cache.md` §Write Rules (Tooled writes). Candidate targets, plus stable-only targets with a usable baseline (rule verify has been removed). Main agent MUST plan the re-run before it starts and finalize it only after every packet is resolved; otherwise `fresh` stays `MISSING`/`BLOCKED` and `promote` is rejected — see `framework/unit_verify_checklist.md` §Completion — Persist Gate Cache. |
| `rereview@{unit}` | Delta: re-review only the files whose evidence went stale + cross-check, carry the rest over; a blocking baseline re-runs the failed files instead. Writes a cache with `mode: full`, `basis: delta` (`repair` from a blocking cache); delta FAIL writes the blocking cache extended with the per-check status map. Writes via the packet-run sequence (`specflowctl gate-plan --mode delta` (or `--mode repair` from a failure record) before the re-run, `specflowctl gate-packet` before every packet executor with its output included verbatim in that executor's prompt, one `specflowctl gate-submit` per required packet report, `specflowctl gate-finalize` after every packet is resolved — `specflow/tooling/bin/specflowctl-<os>-<arch>`, `<tooling-root>` is `specflow/tooling`) — see §Gate Work Packets and `framework/validation_cache.md` §Write Rules (Tooled writes). Candidate targets, plus stable-only targets with a usable baseline. Main agent MUST plan the re-run before it starts and finalize it only after every packet is resolved; otherwise `fresh` stays `MISSING`/`BLOCKED` and `promote` is rejected — see `framework/spec_review_checklist.md` §Completion — Persist Gate Cache. |

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
| `deps@{rule}` | Runs `specflowctl deps --rule <id>` and reports the units bound to the rule. For a bound rule (`b_rule_*`): the units that list it in their `rule_refs`; empty output means no consumers. For a global rule (`g_rule_*`): every current-layer unit — global rules apply to all units by default and are not repeated in `rule_refs` (unlike `consumers@{rule}`, retiring units are excluded — the graph excludes them because their references disappear with them; `consumers` also resolves current-layer files and excludes retiring units for bound rules, while its global-rule branch lists every existing unit file including a retiring candidate's). A global rule with no rule file (removed, mistyped) is reported as not found. |

`deps` has no `full`/`targeted` distinction and no `:keyword` variant — it is a read-only structural report. It only reads spec files; it never writes, deletes, or touches caches or baselines, and it never runs validate/verify/review. It complements `next` (single-unit discovery) and `fresh` (gate freshness) without overlap: when `validate` FAILs a unit on a cycle, `deps@all` is the diagnosis step — the cycle members and the full edge set tell the user what to unbind before re-running validate.

Gate status vocabulary: `FRESH` (cache exists and satisfies the gate), `STALE` (cache exists but files changed, coverage is incomplete, or mode/result is invalid — re-running the gate fixes it), `MISSING` (no cache file — never run, or a validate full-run failure deleted it; verify/review full-run failures write a failure record instead, see §Failure handling by gate role in `framework/validation_cache.md`), `BLOCKED` (a failure record: validate/verify/review cache declares P0/P1 findings), `OK` (appendix gate: all appendices are covered by the validate cache).

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

## Gate Work Packets

A gate run is a deterministic set of **work packets** generated and persisted by `specflowctl gate-plan`. The packet is the unit of decomposition, tracking, and retry; the run's execution topology is not part of the model.

1. **Deterministic generation.** `gate-plan` fixes the run's input snapshot and generates the packet set from the gate, target, layer, and mode — the same inputs always produce the same packet ids and scopes (see §Packet generation rules). Every packet id is non-empty and unique within the run; duplicate acceptance-item ids, reserved-id collisions, missing dependencies, duplicate dependencies, and self-dependencies reject the plan before any run state is written.
2. **Independent completion.** Each packet is a self-contained work unit: it carries its check keys, its read surface, its dependencies, and its report schema. Any executor can complete it — one agent, several parallel workers, a human, or CI — provided it is an independent context that does not hold the context in which the spec was written (see §Guarantee Boundary).
3. **Mechanical tracking.** Packet state lives in the run state under `meta/gate_runs/<run_id>/` (see `framework/validation_cache.md` §Write Rules → Tooled writes). A packet id is a logical identity, not a filesystem name: each packet state filename is the lowercase hexadecimal SHA-256 of the complete UTF-8 packet id plus `.json`, while the original id remains embedded in and validated against the state object. This single fixed-length mapping is valid on every supported platform, including Windows, and prevents long review paths or reserved filename characters from entering the state path. `specflowctl gate-submit` validates each report mechanically, parses it once into an immutable `packet_result`, and records the report text, parsed verdicts/findings/scopes/severity confirmations, and digest together. A rejected packet can be re-submitted; every attempt is kept. An accepted packet is terminal. Every mutating gate command enters the same repository-local operating-system lock before it loads mutable run state and holds that lock until the whole transition is committed. This makes plan replacement, terminal packet acceptance, and finalize publication linearizable across independent processes while leaving packet execution itself parallel. Finalize therefore either publishes before a later plan replaces its run, or loads after replacement and fails because the old run no longer exists; a replaced in-memory run cannot publish afterward.
4. **Formal verify analysis.** A verify plan contains a detection packet and an analysis packet for every planned item. Detection `MISMATCH` makes its analysis packet required; `ALIGNED` or `CANNOT_DETERMINE` marks it `not_required`. A cross packet is not ready until every analysis packet is either `accepted` or `not_required`.
5. **Result-consuming cross-check.** `specflowctl gate-packet --run <id> --packet <id>` materializes the packet context. For `cross` it includes every accepted packet result and every carried baseline judgment, with digests. Cross submission binds those exact digests into its accepted state and must dispose every input finding and publish an effective status for every logical judgment.
6. **Resume from disk.** Run progress and parsed results are readable from run state alone: `specflowctl gate-status` reports every packet's status, attempts, and next action, so an interrupted run resumes without conversation context. Run identity is path-bound: the requested run id, containing directory, and embedded `run_id` must match the tooling-generated id form exactly, and every state path must remain under `meta/gate_runs/`; malformed identity fails before any write or cleanup.
7. **Mechanical completion.** `gate-finalize --run <id>` accepts no judgment flags. It derives the gate result, blocking state, severity counts, and per-check statuses from the accepted synthesis result (or the single rule-validate packet), then writes the cache only when the input snapshot is unchanged and the assembled evidence covers the run plan.

### Packet record

| Field | Meaning |
|-------|---------|
| `packet_id` | Non-empty stable identifier unique within the run (e.g. `structural`, `detect:{item}`, `analysis:{item}`, a reviewed file path, `cross`); `cross` is reserved for the single cross packet |
| `kind` | The packet's role: `checks`, `item` (verify detection), `analysis` (verify Step 7), `file`, or `cross` |
| `check_keys` | The check keys this packet's report must carry verdicts for (validate: the check number `{n}` — the checklist's `{n}. <check name>:` verdict line; verify: the item id; review: the file path; cross: `cross`) |
| `depends_on` | Packet ids that must be resolved before this packet can be submitted; `accepted` satisfies every dependency and `not_required` satisfies an optional analysis dependency |
| `read_refs` | The exact snapshot entries this packet may declare. `gate-submit` validates against this packet-local set, not the run-wide snapshot |
| `status` | `pending` → `accepted`, `rejected`, or (verify analysis only) `not_required` |
| `attempts` | One entry per submission: attempt number, timestamp, `result_digest`, status, rejection reason |
| `report` | The accepted report, stored verbatim for presentation/audit |
| `packet_result` | The immutable parsed verdicts, findings, dependency scopes, severity confirmations, and report digest used by later packets and finalization |
| `consumed_result_digests` | For result-consuming packets, the exact dependency result digests bound at acceptance |

### Packet generation rules

| Target | Full | Delta (stale recovery) | Repair (failure-record recovery) |
|--------|------|------------------------|----------------------------------|
| `validate@unit` | `structural` (checks 1, 3, 6), `design` (2, 4), `acceptance` (5), `dependencies` (7, 8), `cross` | the packets owning the affected check groups + the groups owning the current keys the baseline never declared + `cross`; the rest are carried (recorded in the run's `carried_keys`) | the packets owning the failed checks + the newly affected groups + the groups owning the current keys the baseline never declared + `cross`; the rest carried |
| `validate@rule` | one packet owning all 8 checks | the packets owning the affected checks + the checks the baseline never declared; the rest carried (rules have no cross-check) | the packets owning the failed + newly affected + never-declared checks; the rest carried |
| `verify@unit` | `detect:{item}` + `analysis:{item}` per acceptance item, then `cross`; analysis is conditionally required by the detection verdict | the detection/analysis pairs of stale items and of items the baseline never declared + `cross`; the rest carried | the detection/analysis pairs of failed/newly affected/never-declared items + `cross`; the rest carried |
| `review@unit` | one packet per reviewed code file + `cross` | the packets of the stale files and of files the baseline never declared + `cross`; the rest carried | the failed/newly affected/never-declared files + `cross`; the rest carried |

The delta re-run set is the complete definition: the checks whose declared deps went stale ∪ the current keys the baseline never declared ∪ explicit `--rerun` keys ∪ the cross-check (unit targets); repair additionally includes the failure record's `fail` judgments and persisted `invalidated_checks` — they are re-run reasons and need no stale source (see §Failure recovery). A key the baseline never declared — a new acceptance item, a review file added since the baseline — has no evidence to carry, so it must execute exactly like a stale judgment. When the re-run set (after group expansion) covers every declared check, nothing is carried over: the plan is the full packet set for the declaration and is reported as a full-scope re-run.

Stable-only targets use the same rules with `target: stable`. Rule targets have no cross-check and no verify/review. Analysis packets are planned up front so the run graph stays deterministic; no packet is added after execution begins.

### Input roles

The derived target surface and `--input` have different roles:

- spec-derived `implementation_surface` / `affects.files` entries define verify evidence and the review file packet set;
- `--input` adds evidence entries that every packet may read and declare, but it never creates a check, item, or review-file packet;
- file, directory, and logical-reference forms of `--input` have the same semantics; directories expand into evidence files only. Every physical path, including spec-derived paths, must resolve inside the project root; absolute external paths, lexical `..` escapes, and in-project symlinks that resolve outside the project are rejected before run state is written;
- cross receives the complete snapshot plus all accepted/current and carried judgment results.

This separation is part of the interface. A report declaration that belongs to the run snapshot but not to the submitting packet's `read_refs` is rejected.

**Delta and repair scope are mechanism-derived.** `gate-plan --mode delta` reads the stale cache's per-check evidence via the same mechanism `fresh@` uses (`DELTA SCOPE`): the re-run set is the checks whose declared deps went stale, the current keys the baseline never declared (a new acceptance item or review file has no evidence to carry, so it must execute), explicit `--rerun` keys, and the cross-check. A stale dependency no check declared is mapped by fixed association where one exists (on a unit target: a `unit:` entry → Check 7; a `rule:` entry → Check 8; on a rule target: a `unit:` entry → the consumer-discovery Checks 5/7 — see §Incremental scope); where no fixed association exists or the cache carries no per-check evidence, the plan degrades conservatively to the full packet set and reports the degradation. `gate-plan --mode repair` additionally reads the failed judgments from the failure record's `status` map and the targeted contradictions from its machine-owned `invalidated_checks`. When the re-run covers every declared check, nothing is carried over and the plan is reported as a full-scope re-run ("the re-run covers every declared check — the plan covers the full scope") — the same packet set a full run would produce for the declaration. The executor does not select the scope; it executes the planned packets.

### Packet report contract

Each packet report is the packet-scoped fragment of the command's report. `gate-submit` parses it once into `packet_result`:

- every `check_keys` entry appears exactly once with a verdict token from the command's allowed set (validate: `PASS | WARNING | FAIL`; verify: `ALIGNED | MISMATCH | CANNOT_DETERMINE`; review: the packet file's dimension assessment with its conclusion; cross: `PASS | FAIL`); every validate/verify/cross verdict carries a non-empty reason, and a review `unacceptable` conclusion carries a non-empty reason — the non-blocking review conclusions (`acceptable` / `needs_attention`) take their evidence basis from the packet's six Dimension 8 assessment lines;
- gate-specific packet structure is complete: unit validate Check 5 reports sub-checks `5a` through `5i`; verify detection reports an `evidence` line, a `deterministic` line, `Part A`, and `Part B` (using an explicit `skipped` reason when Part B does not apply); review file packets report `conclusion`, all six Dimension 8 assessment lines with non-empty bases, `gate_findings`, and `Suppressed by spec (N)`;
- every check key declares at least one `Dependency scope:` line whose file resolves inside that packet's `read_refs` and whose declaration parses;
- local findings receive stable run-scoped ids `{run_id}/{packet_id}/F{n}` in report order (the run id is printed in the packet context — the `Run:` line of `gate-packet`);
- verify analysis reports must identify their item and carry root cause, suggested direction, severity, confidence, and dependency scope;
- cross reports must bind every dependency result digest, publish one disposition (`retained`, `suppressed`, or `merged`) for every input finding, publish one effective `pass|fail` status for every logical judgment plus `cross`, describe every cross-created finding together with its affected logical keys, and publish one complete severity-confirmation sequence for every terminal retained finding. Suppression/merge requires a reason; every merge chain must terminate at a retained input finding or a new cross finding (never at a suppressed finding, a missing target, or a cycle); a failed cross rule requires a retained P0/P1 cross finding (the gate result is derived from P0/P1 findings — a `FAIL` verdict backed only by advisory retained findings would contradict the derived result).

The main agent presents the unified report generated from the accepted artifacts. `gate-finalize` derives the run-level header (`Result`, `Blocking promote`, `Key counts`) and cache body; the main agent does not supply or recompute those values.

**Verify analysis report fields:** exactly one `Item: {id}`, `Root cause: ...`, `Suggested direction: ...`, `Severity: P0|P1|P2|P3`, and `Confidence: high|medium|low`, followed by the item's dependency-scope lines. Root cause and direction values are the Step 7 vocabulary in `framework/unit_verify_checklist.md`.

**Cross synthesis lines:** in addition to the command's `Cross-check:` verdict and dependency scopes, cross reports:

```text
Finding disposition: {input_finding_id} = retained
Finding disposition: {input_finding_id} = suppressed — {evidence-backed reason}
Finding disposition: {input_finding_id} = merged -> {retained_finding_id} — {reason}
Effective status: {logical_check_key} = pass | fail
Effective status: cross = pass | fail
[P0|P1|P2|P3] {location} — {new cross finding}
Finding affects: {cross_finding_id} = {logical_key}[, {logical_key}...]
Severity confirmation: {retained_finding_id} = confirmed {Px} — evidence: {read_ref}; reason: {one line}
Severity confirmation: {retained_finding_id} = adjusted {Px} -> {Py} — evidence: {read_ref}; reason: {one line}
# After an adjusted first result, exactly one final second result follows:
Severity confirmation: {retained_finding_id} = confirmed {Py} — evidence: {read_ref}; reason: {one line}
# or
Severity confirmation: {retained_finding_id} = adjusted {Py} -> {Pz} — evidence: {read_ref}; reason: {one line}
```

There is exactly one disposition per input finding and one effective status per logical check/item/file plus `cross`. Bracketed findings in a cross report are new cross findings; each must name at least one affected non-cross logical key. Retained local findings are referenced by disposition instead of copied. A `merged` disposition identifies a duplicate, not a removal: following its target repeatedly must reach a finding that is retained in the final result. A target that is suppressed, missing, self-referential, or part of a merge cycle rejects the cross report. The terminal retained finding is counted once, and its final affected-key set is the union of its own keys and the source keys of every finding merged into it.

Every terminal retained finding — current, carried, or cross-created — has one complete `Severity confirmation` sequence. A first `confirmed` record is final and the sequence contains one line. A first `adjusted` record must be followed by exactly one second record for the adjusted severity; that second record is final and may either confirm the new severity or adjust it by one more adjacent level. A suppressed or non-terminal merged finding has no sequence. Finding ids are already visible in dependency results; a finding created by the report being authored uses the deterministic run-scoped id `{run_id}/{packet_id}/F{n}` from its finding order (for example `20260916-101112-abc123/cross/F1`). The run prefix is the run id printed in the packet context — the executor copies it and must never invent an id. Run-scoped ids are unique by construction across runs: a finding authored by the current report can never collide with a carried finding from an earlier run. Every `confirmed` record must start and end at the finding's current severity. Every `adjusted` record must start at the current severity and move to the immediately adjacent level; its result becomes the starting severity for the next record or the finding's final canonical severity. Each record's evidence path must belong to the cross packet's `read_refs` and must also be covered by a `cross` dependency-scope declaration in the same report. Empty reasons, unknown finding ids, missing or incomplete sequences, records for non-terminal findings, broken severity chains, non-adjacent adjustments, and third records reject the submission.

For every non-cross logical key, effective status is closed mechanically over that retained-finding set: it is `fail` if and only if at least one retained P0, P1, P2, or P3 finding affects the key, and is `pass` otherwise. `Effective status: cross` must exactly mirror the cross verdict. `gate-submit` binds the dependency digests automatically from run state, so the report does not transcribe hashes. `gate-finalize` reapplies the accepted severity confirmations to the resolved terminal finding set and derives counts, blocking, result, and the carried judgment baseline only from those canonical severities.

### Execution topology

Packets define what must be done and how completion is tracked; they do not constrain who executes them or in what order beyond `depends_on`. The framework's independence requirement is an execution property — the executor must not hold the context in which the spec was written — not a topology: a run may be executed by one worker sequentially, by parallel workers, or by a coordinator aggregating independent workers. No run or packet field records session, worker, or platform identity; the tooling verifies content and state, never execution shape (see §Guarantee Boundary).

## Guarantee Boundary

The gate chain combines two property classes. Only the first is mechanically checkable by the runtime-neutral tooling:

**Core-mechanical guarantees** — verified from artifacts at gate-plan, gate-submit, gate-finalize, `fresh`, and `promote` time:

1. **Input snapshot consistency** — a cache is written only by a gate run whose input snapshot (fixed at `gate-plan`, before execution) was byte-unchanged at `gate-finalize`; every declared file or region resolves to the recorded content CIDs, compared byte-for-byte
2. **Coverage** — every required check key, acceptance item, or review file is present in the cache (validate additionally requires every non-exempt appendix). Verify analysis packets are accepted or mechanically `not_required`; cross covers every current and carried judgment and every finding
3. **Output structure** — the report and cache carry the required fields per the gate's own checklist
4. **Result closure** — cross is bound to the accepted dependency-result digests; `gate-finalize` derives result, blocking, counts, and statuses without coordinator-supplied judgment
5. **Cache closure** — the derived result is persisted and checked through the fresh → promote chain

**Runtime-dependent properties** — the required execution shape, not observable from repository artifacts:

1. the run executes in an independent context — the executor does not hold the context in which the spec was written
2. a real separate worker executes it — not the main agent itself
3. the executor is read-only — no file modification, no state-changing commands, no further sub-agent launch

Session, worker, and permission are runtime-internal concepts; runtime-neutral tooling cannot observe them. A marker written by the run itself (`session_id`, `worker_id`, `read_only: true`) is a self-report, and the writer is the party whose independence is in question — self-reported execution shape is not evidence and must not enter any cache or report. The independence requirement remains in force as an execution policy: it is what keeps a verdict from being self-approval, but it is a premise of judgment quality, not a mechanically proven premise of the gate. No report may claim "independence verified".

**Declared vs attested:** independent execution is **declared** — required by the framework rules and held by executor discipline; this is its status today. It would be **attested** only if a signature from a provider the core trusts covered the execution facts. No attestation provider exists today, and none is required for the gates to run. If stronger assurance is ever needed, the only admissible mechanism is a runtime-neutral, property-based interface — input snapshot, executor isolation, write capability, output digest, provider, signature — implemented by runtime or CI adapters; core verifies trusted provider signatures only. A platform-specific session field or unsigned metadata is never `attested`.

## Sub-agent Prompt Assembly

The sub-agent prompt assembly structure is shared by the three quality-gate commands. The main agent assembles the prompt from the target's files and the command's own checklist, following this fixed structure. Command-specific values are listed per field below.

The three commands execute **one packet per independent read-only session** (see §Gate Work Packets). Before assembling the prompt, the coordinator runs `specflowctl gate-packet --run {run_id} --packet {packet_id}` and includes that output verbatim as the packet context; this is how analysis and cross executors receive the accepted results they consume. The executor never holds the context in which the spec was written. Independence is a required execution shape, declared but not mechanically verifiable (see §Guarantee Boundary); the tooling verifies the submitted artifact and its bound inputs, not who executed it.

A sub-agent prompt is a **mission package for a zero-context worker**: the sub-agent has no conversation history and no framework knowledge beyond this prompt and the files it is told to read. The prompt must therefore answer five questions without requiring inference — who am I, what am I doing, why, what counts as done, and who consumes my output. The mandatory fields below guarantee this.

**Mandatory fields:**

1. **Role line** —
   - validate: "read-only validation sub-agent for {unit|rule} {name}, packet {packet_id} of run {run_id}", followed by "You are an independent read-only session: you do not hold the context in which this spec was written, and the main agent does not re-litigate your verdicts — it collects your output verbatim into the final report."
   - verify: "read-only detection sub-agent for verify packet {packet_id} of run {run_id}", followed by "You are one of the run's independent read-only workers; the main agent collects all packet results verbatim into the final report."
   - review: "read-only review sub-agent for review packet {packet_id} of run {run_id}", followed by the same independent-worker sentence.
2. **Mission statement** — one sentence stating the deliverable, fixed form:
   - validate: "Validate {target} per the protocol: execute each check in Packet scope, report PASS / WARNING / FAIL per check with a reason (FAIL reasons identify the contradicting information sources), and report findings with P0/P1 severity and resolution type."
   - verify: "Verify each item in Packet scope: decide ALIGNED / MISMATCH / CANNOT_DETERMINE per the protocol, with deterministic evidence for every claim."
   - review: "Review the file in Packet scope per the protocol and report findings with P0-P3 severity."
3. **Spec source** — main spec path + version, plus the appendix directory (all non-exempt, non-retired appendices are part of the spec, per the checklist prerequisites)
4. **Check / Packet scope** —
   - validate: the packet's check keys (e.g. the `structural` packet owns checks 1, 3, 6; the `cross` packet owns the cross-check); the packet record is authoritative — read it with `specflowctl gate-status --run {run_id}`; validate targeted (`:check-{n}` / `:{keyword}`): executed directly by the main agent, no sub-agent is launched and no packet is generated (see §Targeted Runs)
   - verify: the packet's acceptance item id (the item id itself is the anchor — not line numbers; the spec is a moving target)
   - review: the packet's file path (the file itself is the anchor)
   - **Delta / repair runs:** the prompt's Check / Packet scope lists the packet's re-run keys only; the sub-agent reports `Dependency scope` for the checks it actually executed — carried-over checks are not re-executed and get no new scope declaration (their evidence stays in the cache unchanged). The packet set of a delta/repair run is planned by `gate-plan` (see §Delta Runs).
5. **Read surface** — the files the sub-agent may need:
   - validate (unit): packet-local inputs are derived from the checks the packet owns. Every packet receives the unit's own spec and appendices, explicit `--input` evidence, and the shared `affects.files` evidence surface; `structural` additionally receives the resolved `unit_refs` / `rule_refs` needed by Check 1, and `dependencies` receives those logical references for Checks 7-8. Implementation code is not read
   - validate (rule): the candidate rule file, its stable sibling if present (Check 4), and the unit spec files under `docs/specs/units/` (Check 5 existence check and Check 7 consumer discovery)
   - verify / review: from `implementation_surface` and `affects.files`
6. **Protocol reference** — the command's own checklist, as the only protocol source: validate unit → `framework/unit_validate_checklist.md` (all 8 checks, cross-check included); validate rule → `framework/rule_validate_checklist.md` (all 8 checks, no cross-check); verify → `framework/unit_verify_checklist.md` Steps 1-6 (Step 2's Part A/B sub-checks included); review → `framework/spec_review_checklist.md`
7. **Context** — the background the sub-agent needs to orient itself, four fixed sentences: which command and target this run serves ("You are part of the `validate@{unit}` full run (packet {packet_id})"); execution-shape declaration (validate: "You are an independent read-only session — your verdicts are not self-approval, and the main agent collects them verbatim; packet boundaries are deterministic — your result is the same whether the run executes packets sequentially or in parallel"; verify/review: "Packet boundaries are deterministic — your result is the same whether the run executes packets sequentially or in parallel"); output consumption ("Your output is collected verbatim by the main agent into the final report; follow the protocol's output format exactly"); unit orientation ("{unit} is {one-sentence description}; candidate version {version}"). The one-sentence unit description is distilled from the spec's goal/responsibility sections (spec path from `specflowctl next` output); the version comes from the spec frontmatter.
8. **Glossary** — one line per term used in this prompt or in the protocol steps this sub-agent executes. Each line: term — one-sentence definition — source reference. The canonical glossary below is the full set for the three quality-gate commands; the main agent includes every term that appears in the assembled prompt and no others. Scenarios outside the three commands (e.g. the `spec_flow_issues` triage prompt) define their own glossary in their protocol file (`framework/operations/issues.md` Step 3).
9. **Permissions** — verbatim: "You may read files, search text by pattern, glob for files, and run read-only git queries. You must NOT modify any file, run any command that changes state, or launch further sub-agents."

   **Assembly rule — tooling binary path:** When the main agent assembles the sub-agent prompt, it MUST resolve `specflow/tooling/bin/specflowctl-<os>-<arch>` to the concrete binary for the current execution environment (e.g. `darwin-arm64`, `linux-amd64`, `windows-amd64.exe` — list `specflow/tooling/bin/specflowctl-*` to find the installed binary; `uname -s`/`uname -m` mapping is the fallback). The placeholder `<os>-<arch>` MUST NOT appear in the assembled prompt. The sub-agent MUST NOT attempt to infer or construct the tool path.

**Canonical glossary** (source references are authoritative; a definition is a locating aid, not a rule restatement):

| Term | Definition | Source |
|---|---|---|
| acceptance item | An entry in the spec's `acceptance_item_set` (in the `Testability / Acceptance Criteria` section), carrying id, description, verification_type, pass_condition and other fields | `framework/spec_writing_guide.md` §7 |
| pass_condition | The condition an item must satisfy, written as verifiable assertions (e.g. "Returns HTTP 201") | `framework/spec_writing_guide.md` §7 (Acceptance Item Fields) |
| verification_type | The item's verification mode: `testable` (automated test), `inspectable` (file/artifact inspection), `reviewable` (human review) | `framework/spec_writing_guide.md` §7 (Acceptance Item Fields) |
| implementation_surface | The per-item code surface path the item's implementation lives under; `<pending>` is a placeholder that verify reports as MISMATCH | `framework/spec_writing_guide.md` §7 (Acceptance Item Fields) |
| affects.files | The implementation files an item declares as its scope for verify | `framework/spec_writing_guide.md` §7 (Acceptance Item Fields) |
| unit_refs | The frontmatter declaration of units this unit depends on (formal behavior contract); validate Check 7 reads the referenced units' contracts | `framework/spec_writing_guide.md` §4 (Unit Dependencies) |
| rule_refs | The frontmatter declaration of rules bound to this unit; validate Check 8 reads the referenced rules | `framework/spec_writing_guide.md` §5 (Rule References) |
| candidate / stable layer | The spec layers: candidate is the working draft, stable is accepted truth; the layer is encoded by the file path | `framework/concepts.md` §The Two Layers |
| ALIGNED / MISMATCH / CANNOT_DETERMINE | The per-claim verdicts; fold order MISMATCH > CANNOT_DETERMINE > ALIGNED | `framework/unit_verify_checklist.md` §Verdict folding |
| deterministic evidence | A reproducible static check: a grep command with its result, a file existence check, or a file:line read | `framework/unit_verify_checklist.md` Step 2 |
| Dependency scope | The report's per-check dependency declaration: one line per check stating the file and the section-region headings (or line ranges, acceptance item regions, or `all`) that check's judgment depended on, recorded by `gate-finalize` as the cache's per-check `checks` mapping | `framework/unit_verify_checklist.md` §Output Format |
| section region | A content region of a markdown file located by heading: the frontmatter region (file head through the line before the first `##` heading, heading `""`) or one `##` heading section (heading line through the line before the next `##` heading; deeper headings belong to their `##` section). Declared as `region:section:<heading>:<cid>` | `framework/validation_cache.md` §Structural Region Dependencies |
| structural region | Content located by structure rather than line numbers: the whole acceptance item set (`region:acceptance_items:<cid>`, an order-insensitive semantic CID over its set preamble and sorted item members), one acceptance item (`region:acceptance_item:<id>:<cid>`, from its `- id:` line to the next item), or a section region; edits outside the declared structure do not stale it | `framework/validation_cache.md` §Structural Region Dependencies |
| Part A / Part B | The two parts of the test design sub-check: coverage completeness / test meaningfulness | `framework/unit_verify_checklist.md` Step 2 |
| B1-B6 | The six Part B checks: mock density, assertion authenticity, tautological assertions, all-happy-path, mock-through, test naming | `framework/unit_verify_checklist.md` Step 2 |
| stub | A placeholder or debt marker in implementation code (Step 6 grep findings; RELEVANT hits are MISMATCH) | `framework/unit_verify_checklist.md` Step 6 |
| surplus | Code structure with no spec correspondence (reported as MISMATCH type: surplus) | `framework/unit_verify_checklist.md` Step 5 |
| structural / acceptance / scope | The remaining MISMATCH type values: structural (Step 1 declaration mismatch), acceptance (Step 2 pass_condition mismatch), scope (Step 3 affects declaration mismatch) | `framework/unit_verify_checklist.md` Steps 1-3 |
| severity (verify / review) | The P0-P3 grade of a finding; blocking semantics per the shared severity policy | `framework/severity_policy.md` §4 |
| fact anchor | A reproducible repository fact that proves a P3 discrepancy by identifying its governing or comparison reference, violating location, and relationship | `framework/spec_review_checklist.md` §5 P3 Reportability Gate |
| P3 reportability gate | The review gate that permits a P3 finding only when its discrepancy is objective, local, low-impact, and supported by a reproducible fact anchor | `framework/spec_review_checklist.md` §5 P3 Reportability Gate |
| §9 severity confirmation | The severity consistency check the cross executor records before its synthesis result is accepted | `framework/severity_policy.md` §9 |
| cross-check | The final consistency check over all content after individual checks pass | `framework/verification_scope.md` §Cross-check |
| check 1-8 (validate) | The eight validate checks: structural integrity, design soundness, scope integrity, evidence-driven vs design-driven consistency, acceptance coverage & correctness, affects-source validity, cross-unit consistency, constraint alignment | `framework/unit_validate_checklist.md` |
| exempt / retired appendix | Appendix statuses skipped when assembling the spec union (`status: exempt` / `status: retired` files are not read) | `framework/unit_validate_checklist.md` §Prerequisite |
| resolution type | Each finding's fix classification: `actionable` (concrete repair without user judgment) or `needs_decision` (requires user input) | `framework/unit_validate_checklist.md` §Execution Rules |
| advisory finding | validate's non-blocking items: Check 1 step 7 / step 13 hygiene WARNING and Check 2 Step 4 taste-level P2/P3, presented on the check line's reason, counted separately, never blocking | `framework/unit_validate_checklist.md` §Counting rules |
| extraction artifact | The falsifiable evidence carried by uncovered-domain (5a) and uncarried-contract (5h) FAIL findings, with three parts: the quoted source declaring the behavior domain or contract statement, the granularity/external-visibility judgment, and the absence claim over the covered surface (the item set union / the carriers) | `framework/unit_validate_checklist.md` 5a step 8 / 5h step 4 |
| external-visibility boundary | The §4 judgment separating contract statements (externally-observable behavior a caller or dependent unit must read to use the unit safely) from internal design expression (internal field names, internal field layouts, internal timing incl. retry/backoff values, internal data structures and their operations, internal function behavior, configuration layout); content on the internal side creates no coverage or carrier obligation | `framework/spec_writing_guide.md` §4 |
| P0 / P1 severity (validate) | The only severities validate grades: P1 is the contract-decided default for FAIL checks (recorded as `confirmed` without a §9 boundary check); P0 requires the §9 boundary check | `framework/unit_validate_checklist.md` §Severity check |
| Dimension 8 (module_boundaries / responsibility_organization / dependency_clarity / abstraction_level / extension_landing_points / engineering_patterns) | The architectural design quality assessment of the reviewed code surface: per-packet lines reporting module boundaries, responsibility organization, dependency clarity, abstraction levels, extension landing points, and engineering patterns, each with an assessment and basis | `framework/spec_review_checklist.md` §4 (Dimension 8) / §Body format |
| spec_context | The finding's attached relevant design context from the spec, helping the user understand the code-design relationship | `framework/spec_review_checklist.md` §Findings section / §6 |
| recommendation | The finding's fix suggestion | `framework/spec_review_checklist.md` §Findings section / §6 |

**File-list baseline:** the main agent runs `specflowctl next --unit <name>` and uses its output (spec file, appendices, implementation surface, affects files, acceptance item ids) as the mechanical baseline for fields 3-5. Test files are collected by globbing `*_test.go` next to each implementation file — never by guessing. For validate, the same command output supplies the spec, the appendix directory, and the dependency targets (`unit_refs` / `rule_refs`) that make up the read surface. For `validate@{rule}` — `specflowctl next` supports unit targets only — the mechanical baseline is the command target file `docs/specs/rules/candidate/{rule_id}.md`, its stable sibling `docs/specs/rules/stable/{rule_id}.md` if present (Check 4), and the unit spec files globbed under `docs/specs/units/` (Checks 5/7).

An `implementation_surface` value of `<pending>` produces no file-list entry — the placeholder declares an unknown implementation surface; its judgment belongs to verify Step 6 (MISMATCH), it is never collected as a file.

**Prohibitions:**

1. No inline restatement of protocol rules (severity tables, evidence thresholds, Part B check definitions) — the sub-agent reads the checklist itself. This prohibition covers protocol rules only: the context declarations above (fields 1-9) are mandatory and are not restatements — they locate the protocol, they do not replace it. When any prompt text conflicts with the protocol, the protocol wins and the sub-agent reports the conflict.
2. No severity assignment outside the command's own rules — validate sub-agents grade findings P0/P1 with a resolution type per `framework/unit_validate_checklist.md`; verify detection sub-agents report MISMATCH type only and Step 7 analysis packets assign severity/direction; review sub-agents grade findings P0-P3. The cross executor performs the required §9 confirmation while synthesizing retained findings; the coordinator never re-grades them.
3. No behavior summary presented as normative — the spec is the only normative source; a prompt summary that conflicts with the spec is reported as a prompt/spec discrepancy, spec wins
4. No cache or run-state writes by sub-agents — sub-agents MUST NOT write cache files or run state and MUST only report findings plus `Dependency scope:` lines; the main agent plans the gate run before any executor reads input (`specflowctl gate-plan`), materializes every packet's context and includes it verbatim in that executor's prompt (`specflowctl gate-packet`), submits each packet report verbatim (`specflowctl gate-submit`), and assembles the cache at `specflowctl gate-finalize` (`specflow/tooling/bin/specflowctl-<os>-<arch>`, `<tooling-root>` is `specflow/tooling`, resolved to the concrete binary by the main agent — see `framework/validation_cache.md` §Write Rules → Tooled writes). The declaration schema has no `hash`/`deps` fields — a transcribed CID cannot enter a cache file. The sub-agent MUST NOT attempt to infer or construct the tool path.

**Required output fields:**

- validate per check: `{n}. {check name}: PASS | WARNING | FAIL — reason`; unit Check 5 additionally reports every sub-check `5a` through `5i` with its own verdict and reason; FAIL reasons identify the contradicting information sources per the checklist's Execution Rules; findings use the unified format `[{P0|P1}] {location} — {issue} (actionable | needs_decision)`; cross-check line for unit full runs
- validate extraction evidence: sub-check 5a uncovered-domain and sub-check 5h uncarried-contract FAIL findings must include the extraction artifact defined in the checklist (5a step 8 / 5h step 4) — the quoted source declaring the behavior domain or contract statement, the granularity/external-visibility judgment, and the absence claim over the covered surface (the item set union / the carriers) — so the main agent can re-verify the claim before presenting the finding
- verify per item: `{item.id}: ALIGNED | MISMATCH (type) | CANNOT_DETERMINE — {code references or explicit determination gap}`, followed by exactly one non-empty `evidence:` line, one `deterministic: true|false` line, one `Part A:` result, and one `Part B:` result (an explicit `skipped — {reason}` is the result when Part B does not apply); detection sub-agents do not assign severity; Step 7 analysis packets grade mismatches and cross confirms retained grades before finalization; review findings follow the review checklist's output format, with a `fact_anchor` on every P3 finding
- review packet assessment: each review packet reports exactly one `conclusion`, the six Dimension 8 assessment lines (`module_boundaries`, `responsibility_organization`, `dependency_clarity`, `abstraction_level`, `extension_landing_points`, `engineering_patterns`, each with a non-empty assessment and basis), one `gate_findings` line, and one `Suppressed by spec (N)` block for the file in its packet, per `framework/spec_review_checklist.md` §Body format; the main agent aggregates the per-packet assessments into the final report's Architecture assessment block
- `Dependency scope:` one line per check the run executed — `{check key}: {file}: {declaration}` where `{check key}` is the command's check identifier (validate: the check number `{n}` — a scope line may spell it `check-{n}:`; verify: the acceptance item id; review: the packet's file path), `{file}` is the file the judgment read, and `{declaration}` is the section-region heading text the check's judgment read (e.g. `Description`, `Testability / Acceptance Criteria`, or the frontmatter region as `frontmatter`), `acceptance_item:<id>[,<id>...]` (specific item regions — the verify item judgment's declaration for its own spec block), the reserved token `acceptance_items` (the order-insensitive whole item set), 1-based closed line ranges, or `all` for whole-file judgments. A judgment that depends on item presentation order must declare a containing section, ranges, or `all`, not `acceptance_items`. This is the sub-agent's only evidence declaration — it MUST NOT write `hash`/`deps` values. The packet report carries the declaration; `gate-submit` validates it against that packet's `read_refs` (not merely the run snapshot) and `gate-finalize` computes the CIDs when it assembles the cache's per-check `checks` mapping (see the unified report skeleton and `framework/validation_cache.md` §Format). Delta runs report the scope of the re-run checks only (see Check / Packet scope above)
- failure path: "Validation could not complete — {reason}" (verify: "Verification could not complete — {reason}"; review: "Review could not complete — {reason}")

**Assembly example (verify)** — a complete assembled verify prompt. `{unit}`/paths are placeholders the main agent fills in; the glossary shows only the terms used in this prompt:

```
You are a read-only detection sub-agent for verify packet AUTH-AC-003 of run
20260916-101112-abc123. You are one of the run's independent read-only workers;
the main agent collects all packet results verbatim into the final report.

Mission: Verify the item in Packet scope: decide ALIGNED / MISMATCH /
CANNOT_DETERMINE per the protocol, with deterministic evidence for every claim.

Spec source: docs/specs/units/candidate/unit_auth.md (candidate, version 0.2.1);
appendices: docs/specs/units/candidate/appendix/unit_auth_*.md (non-exempt,
non-retired only).

Packet scope: AUTH-AC-003 (login section).

Implementation surface: src/api/login.go, src/api/token.go, src/store/session.go.

Protocol: framework/unit_verify_checklist.md Steps 1-6 (including Step 2's
Part A/B sub-checks). This is the only protocol source — follow it exactly.

Context: You are part of the `verify@auth` full run (packet AUTH-AC-003). Packet
boundaries are deterministic — your result is the same whether the run executes
packets sequentially or in parallel. Your output is collected verbatim by the
main agent into the final report; follow the protocol's output format exactly.
auth is the user authentication unit; candidate version 0.2.1.

Glossary:
- acceptance item — an entry in the spec's `acceptance_item_set`, carrying id,
  description, verification_type, pass_condition (spec_writing_guide.md §7)
- pass_condition — the condition an item must satisfy, written as verifiable
  assertions (spec_writing_guide.md §7)
- verification_type — the item's verification mode: testable / inspectable /
  reviewable (spec_writing_guide.md §7)
- implementation_surface — the per-item code surface path (spec_writing_guide.md §7)
- affects.files — the implementation files an item declares as its scope
  (spec_writing_guide.md §7)
- candidate / stable layer — candidate is the working draft, stable is accepted
  truth; the layer is encoded by the file path (concepts.md §The Two Layers)
- exempt / retired appendix — appendix statuses skipped when assembling the spec
  union; non-exempt, non-retired files are read (unit_validate_checklist.md §Prerequisite)
- ALIGNED / MISMATCH / CANNOT_DETERMINE — per-claim verdicts; fold order
  MISMATCH > CANNOT_DETERMINE > ALIGNED (unit_verify_checklist.md §Verdict folding)
- deterministic evidence — a reproducible static check: a grep command with its
  result, or a file:line read (unit_verify_checklist.md Step 2)
- Dependency scope — the files and regions your judgment depended on: the
  item's own region (`acceptance_item:{item.id}`) for the unit's own main
  spec, section headings, or per-file line ranges
  (unit_verify_checklist.md §Output Format)
- Part A / Part B — the two parts of the test design sub-check
  (unit_verify_checklist.md Step 2)
- mock density / B1-B6 — Part B's six checks (unit_verify_checklist.md Step 2)
- stub — placeholder or debt marker in code (unit_verify_checklist.md Step 6)
- surplus — code structure with no spec correspondence (unit_verify_checklist.md Step 5)

Permissions: You may read files, search text by pattern, glob for files, and
run read-only git queries. You must NOT modify any file, run any command that
changes state, or launch further sub-agents.

Required output:
- Per item: {item.id}: ALIGNED | MISMATCH (type) | CANNOT_DETERMINE — with code
  references and deterministic evidence (grep command + result, or file:line
  reads); type is the detection verdict (structural / acceptance / scope /
  stub / surplus); severity is not reported by detection sub-agents — the Step 7
  analysis packets assign it and the cross executor confirms retained grades per §9; plus
  Part A and Part B findings per acceptance item
- Dependency scope: one line per item aligned — {item.id}: {file}: {declaration}
  where the item's own spec block is declared as `acceptance_item:{item.id}`
  (the item region, located by id), and additional reads are declared as a
  section heading, line ranges, the whole-set token `acceptance_items`, or
  "all" (the regions of the spec the item's judgment depended on)
- If you could not complete: "Verification could not complete — {reason}"
```

**Assembly example (validate)** — a complete assembled validate prompt for a unit full run. `{unit}`/paths are placeholders the main agent fills in; the glossary shows only the terms used in this prompt:

```
You are a read-only validation sub-agent for unit payment, packet structural of
run 20260916-101112-def456. You are an independent
read-only session: you do not hold the context in which this spec was written,
and the main agent does not re-litigate your verdicts — it collects your output
verbatim into the final report.

Mission: Validate unit payment per the protocol: execute each check in Packet
scope, report PASS / WARNING / FAIL per check with a reason (FAIL reasons identify
the contradicting information sources), and report findings with P0/P1 severity
and resolution type.

Spec source: docs/specs/units/candidate/unit_payment.md (candidate, version 1.3.0);
appendices: docs/specs/units/candidate/appendix/unit_payment_*.md (non-exempt,
non-retired only).

Packet scope: checks 1, 3, 6 (structural integrity, scope integrity,
affects-source validity).

Read surface: the spec above plus its referenced dependency files (unit_refs,
rule_refs, affects.files document targets). Implementation code is not read.

Protocol: framework/unit_validate_checklist.md — the checks in Packet scope.
This is the only protocol source — follow it exactly.

Context: You are part of the `validate@payment` full run (packet structural). You
are an independent read-only session — your verdicts are not self-approval, and
the main agent collects them verbatim. Your output is collected verbatim by the
main agent into the final report; follow the protocol's output format exactly.
payment is the payment processing unit; candidate version 1.3.0.

Glossary:
- candidate / stable layer — candidate is the working draft, stable is accepted
  truth; the layer is encoded by the file path (concepts.md §The Two Layers)
- check 1-8 — the eight validate checks: structural integrity, design soundness,
  scope integrity, evidence-driven vs design-driven consistency, acceptance
  coverage & correctness, affects-source validity, cross-unit consistency,
  constraint alignment (unit_validate_checklist.md)
- exempt / retired appendix — appendix statuses skipped when assembling the spec
  union (unit_validate_checklist.md §Prerequisite)
- unit_refs — the units this unit depends on (formal behavior contract); read by
  Check 7 (spec_writing_guide.md §4)
- rule_refs — the rules bound to this unit; read by Check 8
  (spec_writing_guide.md §5)
- resolution type — actionable / needs_decision, the fix classification each
  finding carries (unit_validate_checklist.md §Execution Rules)
- advisory finding — validate's non-blocking items: Check 1 step 7 / step 13
  hygiene WARNING, Check 2 Step 4 taste-level P2/P3; presented on the check
  line's reason, counted separately, never blocking (unit_validate_checklist.md)
- P0 / P1 severity — the only severities validate grades; P1 is the default, P0
  requires confirmation (unit_validate_checklist.md §Severity check)
- Dependency scope — per-file line ranges your judgment depended on
  (unit_validate_checklist.md §Output Format)
- cross-check — the final consistency check over all content after individual
  checks pass (verification_scope.md §Cross-check)

Permissions: You may read files, search text by pattern, glob for files, and
run read-only git queries. You must NOT modify any file, run any command that
changes state, or launch further sub-agents.

Required output:
- Per check: {n}. {check name}: PASS | WARNING | FAIL — reason; FAIL reasons
  identify the contradicting information sources; findings in the unified format
  [{P0|P1}] {location} — {issue} (actionable | needs_decision)
- Cross-check: N/N PASS — per-check results (the `cross` packet only)
- Failed checks: N | Advisory findings: K
- Dependency scope: one line per check executed — check-{n}: {file}: {declaration}
  where {declaration} is a section heading, line ranges, the whole-set token
  `acceptance_items`, item declarations `acceptance_item:<id>`, or "all" (the
  sections/regions of the spec the check's judgment depended on; for the unit's
  own main spec, name the section-region headings, e.g. "Description" or
  "Testability / Acceptance Criteria"; the frontmatter region is "frontmatter";
  Check 5's coverage judgment declares `acceptance_items`; cross-check reports
  its scope like any other line, with check key `cross` (see
  `framework/validation_cache.md` §Format → Per-check evidence)
- If you could not complete: "Validation could not complete — {reason}"
```

## Cross-check

After all local packets and required verify analysis packets resolve, one independent cross packet performs the run's semantic synthesis. It receives the complete source snapshot, every accepted packet result, and every carried baseline judgment. Its purpose is to detect combined errors that cannot be seen from one packet and to produce the only run-level semantic result consumed by `gate-finalize`.

Cross does not repeat every local check. It checks relationships between their conclusions and the shared source material. Its result must:

1. bind the digest of every consumed current result and carried judgment;
2. dispose every input finding as retained, suppressed, or merged, with evidence for suppression/merge; every merge chain must terminate at one retained input or new cross finding, which is counted once with the merged findings' logical keys;
3. publish the effective status of every logical check/item/file plus `cross`;
4. publish any new cross finding with severity, evidence, and affected check keys;
5. cover the complete logical result set, including carried judgments in delta/repair runs.

An omitted input result, finding, or logical status rejects the cross report. A failed cross rule without a new retained P0/P1 cross finding also rejects it. The coordinator cannot replace or override an accepted cross result.

### Verify cross-check

Checks for consistency across different parts of the spec:

| Check | What it looks for |
|-------|------------------|
| Contract consistency | Same API endpoint described differently in different sections? |
| Data definition drift | Field names, types, or enum values inconsistent across sections? |
| State machine coherence | Transition rules from different sections contradict each other? |
| Error code conflict | Same error code assigned to different error conditions? |
| Cross-reference integrity | Section A references a claim or definition in Section B that doesn't exist? |

**Output:** the five per-check results, finding dispositions, effective logical statuses, and any cross-created findings. The summary `Cross-check:` line is derived from the five results.

### Validate cross-check

After the local validate packets resolve:

| Check | What it looks for |
|-------|------------------|
| Design × Constraints | Does the design (Check 2) respect the global constraints and bound rules (Check 8)? |
| Coverage × Scope | Does the acceptance coverage (Check 5) actually prove the declared scope (Check 3)? |
| Cross-unit cohesion | Do individual unit decisions (Check 7) align with the combined design intent? |

**Output:** the three per-check results plus the same complete disposition/effective-status synthesis contract. A local failure may be suppressed only when the cross packet demonstrates that it is a false positive against the complete source set; the suppression remains visible in the audit body but is excluded from retained finding counts.

### Review cross-check

Review cross consumes every per-file result and checks cross-file authenticity, cross-document consistency, duplicated/root-cause findings, and severity consistency. It removes false positives through explicit `suppressed` dispositions, merges duplicates through explicit `merged` dispositions, and retains or creates the findings from which the review gate is derived. The coordinator does not perform a second informal finding filter after cross acceptance.

## Delta Runs

Delta runs (`revalidate@{target}` / `reverify@{unit}` / `rereview@{unit}`) restore a stale cache with a partial re-run instead of a full one. They are complete-coverage runs: the judgments whose dependency evidence went stale are re-executed, the rest are carried over from the previous run, and the cache is written with `basis: delta`. Promote trusts a delta cache exactly like a full one — carried-over judgments have unchanged dependency evidence recorded in the cache, which is the same trust basis the gate already uses (see §Relationship with Promote).

The delta scope is **derived by mechanism at plan time**: the cache's per-check evidence (`checks`, see `framework/validation_cache.md` §Format) maps every check to the regions it declared; the stale regions are matched against that map to compute the affected check set, and `gate-plan --mode delta` turns that set into the packet plan. The executor does not select the scope — it reports the planned packets, executes them, and covers the rest by carry-over. The map covers per-check declarations only: entries without a `checks` mapping (logical references, contract files, whole-file declarations, declare-heavy extras) have no check association; their staleness is **unclaimed**, mapped to checks by the fixed associations below (§Incremental scope) — where no fixed association exists the plan degrades conservatively (never silently carried over).

### When delta applies

A delta run has two baseline forms — a **pass baseline** (the stale-cache recovery) and a **failure baseline** (the failure-record recovery, §Failure recovery below):

1. **Pass baseline** — a cache exists for the gate, with `mode: full` and `result: pass` (review: `blocking: false`), and at least one judgment must re-execute: a `files` entry whose declared dependency CID is no longer present, or a current key the baseline never declared. If nothing must re-execute, report "cache is fresh — no incremental re-run needed" and stop. A blocked review cache (P0/P1 findings) is not this baseline — it is the failure baseline instead.
2. **Failure baseline** — a cache exists for the gate as a **failure record** (`result: fail` + `blocking: true`; for validate: written by a delta re-run's FAIL; for verify: written by a delta re-run's FAIL or a candidate full-run FAIL; for review: the blocking cache). The recovery re-runs the failed judgments, persisted targeted invalidations, newly affected judgments, current keys the baseline never declared, and explicit `--rerun` overrides — no stale source is required for failed or invalidated judgments.
3. A MISSING cache has no baseline to carry over from — run the full command instead.

**Coverage is a derived conclusion, not a rule:** the re-run set is the affected checks plus the cross-check plus the current keys the baseline never declared plus persisted targeted invalidations and explicit `--rerun` overrides. When that set (after packet-group expansion) covers **every** declared check, nothing is carried over — the plan is the full packet set for the declaration, reported as a full-scope re-run ("the re-run covers every declared check — the plan covers the full scope"). This is not a special case of own-spec staleness: editing one section of your own spec stales only the checks that declared that section, and the delta plan re-runs exactly those (plus any new keys). Where no scope can be derived at all — the cache carries no per-check evidence, an unclaimed dependency has no fixed association, an invalidated key cannot be mapped to the current judgment surface, or the cache is stale for a cause the declared per-check evidence cannot attribute (e.g. a file entry with no dependency chunks, or the main file missing from the files list) — the planner degrades conservatively to the full packet set and reports the degradation. The same conclusion applies to a failure-record recovery.

### Incremental scope (mechanism-derived)

1. `gate-plan --mode delta` runs the same scope derivation `fresh@` reports (`DELTA SCOPE (<gate>)`): the affected check keys from the cache's per-check `checks` mapping, plus the cross-check. Stale dependencies that no check declared — logical-reference entries (`unit:{name}` / `unit:{name}:appendix:{file}` / `rule:{id}`), contract files, whole-file declarations, and declare-heavy extras in the file-level union — are **unclaimed**; the planner maps them by the command's fixed association:
   - unit `validate`: a `unit:` entry → the packet owning Check 7 (cross-unit); a `rule:` entry → the packet owning Check 8 (constraint alignment).
   - rule `validate`: a `unit:` entry → the packets owning the consumer-discovery checks (Check 5/7).
   - `verify` / `review`: an unclaimed code file or dependency entry has no fixed association — the plan degrades to the full packet set and reports the degradation.
   - When the cache has no per-check evidence (written before `checks` support — `fresh@` reports "no per-check evidence"), the plan degrades to the full packet set and reports it.
2. The plan maps the re-run keys to the packets that own them (see §Packet generation rules) and records `carried_keys` for the declared checks that keep their previous evidence. Current keys the baseline never declared — a new acceptance item or a review file added since the baseline — join the re-run set: they have no baseline evidence to carry, so they must execute exactly like a stale judgment. A targeted run that found P0/P1 records the contradicted key through `gate-invalidate`; `gate-plan --mode repair` reads the persisted `invalidated_checks` list and re-runs that key instead of carrying it over. `--rerun` remains an explicit additional override, not the persistence mechanism for targeted findings.
3. The plan always includes the cross-check packet (unit targets — rules have no cross-check): it is a holistic judgment over all content, and changed shared content can change it. When the cross-check is the only re-run key, the plan is the single cross packet and every declared check is carried over.
4. `gate-plan` reports the plan explicitly before execution begins: the re-run packets with their check keys, the carried-over checks with the statement that their dependency evidence is unchanged, and any degradation or full-scope coverage statement. The executor starts only after the plan is printed; the scope is not negotiated at execution time.

### Execution

- Execute only the planned packets. Carried-over checks are not re-executed: `gate-plan` copies their structured judgments, complete renderable finding details, and evidence from the current judgment-schema baseline into the immutable run, and cross consumes those carried judgments alongside the new packet results. A baseline without the current judgment schema requires a full run. When the run is finalized, every retained carried finding is rendered into the new human-readable findings body exactly once; counts without their corresponding finding detail are invalid.
- Any retained P0/P1 finding writes a **failure record**: validate/verify/review write the cache with `result: fail`, `blocking: true`, tool-derived severity counts, a findings body, `basis: delta` (`basis: repair` when a repair run fails again), and the cross-derived per-check `status` map (`fail`/`pass` for the re-run checks, `carried` for unchanged carried-over checks). Promote must not proceed. Candidate validate cache deletion applies only to a full-run FAIL, never delta/repair.
- On PASS, `gate-finalize` writes the cache with `mode: full`, `basis: delta`, a fresh `timestamp`, and a complete `files` list: accepted packet declarations replace the re-run checks' evidence (new `hash` + `deps` + per-check `checks` breakdown, computed by the tooling); carried-over checks keep their baseline entries (their CIDs are unchanged by construction — they were not stale sources), and their entry's declare-heavy file-level deps (the remainder no check owns) stay in the file-level union — a re-run check's superseded deps are never merged back in. The files list stays complete (including the main spec and every appendix) so the appendix and main-file promote checks keep passing.
- The report uses the standard skeleton with `mode: delta`, an `Incremental scope:` section (re-run checks + carried-over declaration + cross-check — rules have no cross-check), and the finalize note "cache written with basis: delta".

### Failure recovery

A delta or repair run over a **failure record** (`result: fail` + `blocking: true`; validate/verify/review delta FAIL writes it, a repair FAIL updates it with `basis: repair`, and verify/review candidate full FAIL writes it too — a full-run record with `basis: full`; review's blocking cache is the same shape) restores the pass cache incrementally after the findings are resolved. This is the counterpart of the stale-cache recovery — same command, same trust model, different baseline. The split between which full FAILs write a record and which delete the cache is defined by gate role (see §Failure handling by gate role in `framework/validation_cache.md`); the recovery itself is identical regardless of `basis`.

1. **Scope derivation (plan time)** — `gate-plan --mode repair` reads the record's per-check `checks` entries:
   - **No usable status map on a failure baseline** — a record that declares no status at all, or a status for only some checks, cannot say which judgments failed, so **nothing may be carried over**: the plan is the full packet set (plus the cross-check for unit targets), reported as a degradation ("the failure record carries an incomplete per-check status map — the plan covers the full scope"). A status map is usable only when every check the record's structured judgment baseline (`GATE_JUDGMENTS`) holds has a status, the status values are exactly `pass`/`fail`/`carried`, and no `carried` appears in a full-run record (`basis: full` has no carried judgments). A key-set mismatch between the status map and the judgment baseline, an unknown status value, and `carried` in a full-run record are malformed state and take the same degradation path — the planner never guesses a missing or unknown judgment outcome. This is the safe path for legacy blocking caches (written before the failure-recovery design) and for any fail/blocking record whose status map is not complete — absent status means **pass** only on a pass baseline, never on a failure baseline.
   - **Complete status map present** — the re-run set = {judgments whose `status` is `fail` in the failure record} ∪ {keys in the machine-owned `invalidated_checks` list} ∪ {judgments whose declared deps went stale against the current content (the fix changed content)} ∪ {explicit `--rerun` overrides} ∪ {the current keys the baseline never declared} ∪ {the cross-check}. `gate-invalidate` writes the persisted invalidation list immediately after a targeted P0/P1 contradicts a recorded `pass`/`carried` judgment. Judgments with `status` `pass`/`carried` are carried over only when their evidence is unchanged and their key is not invalidated. An invalidated key that cannot be mapped to the current judgment surface degrades the plan to the full packet set. A full-run record's status map has only `pass`/`fail` values (every judgment was re-executed by the full run; `carried` appears only in records written by a delta or repair run). The newly stale sources map by the same rules as the stale-scope path; when the re-run covers every declared check, nothing is carried over (see §Coverage above).
2. **Execution** — execute the planned packets. Any P0/P1 finding updates the failure record (fresh evidence, findings body, status map — the record stays). A full pass makes `gate-finalize` write the cache with `mode: full`, `basis: repair`: new evidence for the re-run checks, the original evidence for the carried-over ones, fresh `timestamp`. The plan declares the recovery scope explicitly before execution, as for any delta run.
3. **No stale source required** — the failed judgments are the re-run reason; the "cache is fresh" stop does not apply to a failure baseline.
4. **Precondition** — the failure record must be present and its dependency evidence must still hold for the carried-over judgments (a stale record's carried-over evidence is untrusted: derive the scope from the stale sources per §Incremental scope and re-check conservatively — declare-heavy). A MISSING record is not a baseline: run the full command. A record whose per-check `status` map is absent or incomplete is not a failure-recovery baseline for the status-less judgments — step 1's no-usable-status-map path degrades it to a full re-run (fail-closed: carrying over unresolved P0/P1 findings is not allowed).

The failure record's `basis` records its writer: `delta` for a delta-FAIL record, `repair` for a repair-FAIL record (a recovery run that fails again keeps the record as the failure-recovery baseline), `full` for a full-run record (verify/review candidate full FAIL, stable-only confirmation FAIL). The recovery's pass cache is marked `basis: repair` for audit separation. Promote trusts a `basis: repair` cache exactly like `full`/`delta` — the evidence rules are identical (see §Trust statement).

### Trust statement

A delta or repair cache carries no lower evidence standard than a full cache: every judgment in it is backed by dependency evidence present in the cache at promote time — re-run judgments by their new CIDs, carried-over judgments by their unchanged CIDs. The gate verifies the same mechanical checks for all three. `basis: delta` / `basis: repair` is audit metadata (see `framework/validation_cache.md` §Format); the promote gate does not distinguish full, delta, or repair.

### Layer applicability

Delta re-runs apply to **candidate targets** and to **stable-only targets with a usable baseline**. For a candidate target, a delta run restores promote eligibility, which exists only for the candidate layer. For a stable-only target, a delta run restores the stale confirmation state — it is a recovery, not a complete re-confirmation: it re-runs only the judgments whose dependency evidence went stale and carries the rest over from the pass baseline. The precondition is the same as for candidates — the gate's confirmation cache must exist with `mode: full` and `result: pass` (review: `blocking: false`). A MISSING cache (never confirmed) has no usable baseline — `gate-plan` reports: "No usable confirmation baseline. Run the full `{command}@{target}` (confirmation check) first, or `specflowctl fork {--unit|--rule} {name}` to start a new round." A blocked/failed confirmation cache is a failure record: the failure recovery applies to stable-only targets the same way (restoring the confirmation state with `basis: repair`); when the stable content itself can no longer hold against the changed dependency or rule, the record stays and forking reconciles it (see §Stable-only Targets → On FAIL).

The layer boundary is decided by file existence: a target with a candidate file is a candidate target; a target with only stable files is a stable-only target. A delta run that would apply against the candidate file is valid even when the target's stable counterpart exists — `fresh@` separates the two cache sets with layer-specific checks: the stable validate/verify variants require the stable main spec in the cache's files list (their caches must prove the main file was read), and the stable review variant requires `target: stable` (the review gate has no main-file requirement, so the `target` field carries the layer).

## Stable-only Targets

When no candidate file exists for a unit (or rule), the three full commands run as **confirmation checks** of the stable content's continuing relationship with the outside world. They are read-only — they write a confirmation cache (`target: stable`) and never edit any content; changing consensus content is possible only through `fork` (see `framework/concepts.md` §4). The confirmation checks are the stable counterparts of the candidate gates, but they grant no promote eligibility — their caches are consumed only by `fresh@stable`.

Stable confirmation caches have **two producers** with identical semantics:

1. **A `@stable` confirmation run** — an explicit user-triggered `validate@{target}` / `verify@{unit}` / `review@{unit}` against a stable-only target, writing a fresh `target: stable` cache from that run's judgment.
2. **A successful promote** — `specflowctl promote` rewrites the candidate gate caches into `target: stable` confirmation caches (inverse of the fork rewrite): `target: candidate` → `target: stable`, and every physical path under `docs/specs/.../candidate/` → the `stable/` equivalent (see `framework/validation_cache.md` §Cache lifecycle). The promoted content is byte-identical to the candidate, so the caches' evidence is still valid for the promoted files. This gives every promoted target an immediate stable confirmation baseline and makes the stable-layer delta recovery (`re*`) usable without a full confirmation run first. A retired promote deletes the caches instead (the stable content is gone).

| Command | Relationship confirmed | Cache on PASS | On FAIL |
|---------|----------------------|---------------|---------|
| `validate@{unit}` / `validate@{rule}` | Stable content vs its dependencies and rules (Check 6/7/8: referenced files, cross-unit contracts, global and bound rules) | `target: stable` validate cache | Write a failure record (`result: fail` + `blocking: true`); recommend forking the unit/rule to reconcile the stable content with the changed dependency or rule. The record keeps the confirmation state visible as BLOCKED and is the failure-recovery baseline |
| `verify@{unit}` | Code vs the stable spec (drift confirmation) | `target: stable` verify cache (VERIFIED state) | Write a failure record; report the drift and recommend forking (do not enter divergence resolution — see `framework/unit_verify_checklist.md` §Stable-only mode) |
| `review@{unit}` | Code quality with the stable spec as design context | `target: stable` review cache | Write the cache with `blocking: true` (same as candidate review FAIL); implementation-class defects may be fixed in code and re-reviewed, design-class defects lead to forking |

Rule targets have no verify or review (rule verify has been removed; review is unit-only), so the rule confirmation is `validate@` alone.

The confirmation caches go stale when their dependency evidence changes: the validate cache when a dependency unit's contract, a rule, or a referenced file changes; the verify and review caches when the code changes. Recovery from STALE: a delta re-run (`revalidate@{target}` / `reverify@{unit}` / `rereview@{unit}`) re-runs only the judgments whose evidence went stale and rewrites the cache with `basis: delta` — the default recovery path when a pass baseline exists (see §Delta Runs → Layer applicability). Recovery from MISSING (never confirmed): the full confirmation run of the same command. A blocked/failed confirmation cache is a failure record: recover it with the failure-recovery delta run (`basis: repair`), or fork when the stable content itself no longer holds. The stale signal doubles as impact detection: after a rule change, every stable unit bound to the rule shows `validate: STALE` in `fresh@stable`, naming the impact surface.

## Targeted Runs

Targeted runs are available only through explicit user choice:

- `validate@{target}:check-{n}` — run a single specific check
- `validate@{target}:{keyword}` — run checks matching a keyword
- `verify@{unit}:{keyword}` — verify the spec content matching a keyword
- `review@{unit}:{keyword}` — review the code file matching a keyword

When the user explicitly targets, the agent still reads the **full document** for context but only reports on the requested check(s)/content/file.

**Targeted runs never publish a complete result cache.** They are iterative feedback only, and never satisfy the promote gate. If a targeted run finds P0/P1, the coordinator immediately runs `specflowctl gate-invalidate` with the gate identity and affected check key. The command deletes a matching pass cache; for a failure record it preserves the record and persists the key in `invalidated_checks`, so a later repair plan re-runs it automatically. In the same locked transition it marks a matching open gate run invalidated, preventing an earlier plan from overwriting the new state. The targeted result itself is still not a complete gate result (see §Delta Runs → Failure recovery).

**Targeted execution shape:** targeted runs execute directly in the main agent session — no sub-agent is launched, no prompt is assembled per §Sub-agent Prompt Assembly, and no gate run or packet is generated. They are lightweight single-check feedback outside the promote gate, so the independent-session execution requirement does not apply to them (see §Guarantee Boundary).

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
Incremental scope:
  - check-7 (cross-unit): re-run — dependency unit auth's acceptance item set changed (AC-003)
  - checks 1-6, 8: carried over — dependency evidence unchanged
Cross-check: 3/3 PASS
────────────────────────────────────────────
Dependency scope:
  check-7: docs/specs/units/candidate/unit_user_auth.md: all
  check-7: unit:auth: acceptance_item:AUTH-AC-003
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
| `validate@{target}` full PASS | Write `validate_result.md` with `mode: full`, and `hash` + `deps` dependency evidence for all read files (entries are assembled at `gate-finalize` from the accepted packet reports; see `framework/validation_cache.md` §Write Rules → Tooled writes) |
| `validate@{target}` full FAIL (candidate) | Delete the validate cache (full-run failure — trust establishment failed; no failure record). The spec is the upstream root — nothing may be carried over from an unconfirmed spec (see §Failure handling by gate role in `framework/validation_cache.md`) |
| `validate@{target}` full FAIL (stable-only) | Write a failure record (`result: fail` + `blocking: true`) and recommend forking (see §Stable-only Targets) |
| `verify@{unit}` full PASS (all aligned) | Write `verify_result.md` with `result: pass`, `mode: full`, `blocking: false`, `hash` + `deps` dependency evidence |
| `verify@{unit}` full PASS (P2/P3 non-blocking findings) | Write `verify_result.md` with `result: pass`, `mode: full`, `blocking: false`, severity counts (`p0_count`...`p3_count`), `hash` + `deps` dependency evidence. Promote may proceed |
| `verify` full FAIL (any P0/P1 findings, candidate) | Write a failure record (`result: fail` + `blocking: true`, per-item `status` map — derived mechanically by `gate-finalize` from the accepted packet verdicts) — the `reverify@{unit}` failure-recovery baseline. Agent must stop, not proceed to promote. (Anchored to the validated spec, so `pass` items with unchanged evidence may be carried over — see §Failure handling by gate role) |
| `verify` full FAIL (stable-only) | Write a failure record; report the drift and recommend forking |
| `review@{unit}` full PASS | Write `review_result.md` with `mode: full`, `blocking: false`, `hash` + `deps` dependency evidence for all read files, findings body |
| `review@{unit}` full FAIL (P0/P1 found) | Write `review_result.md` with `mode: full`, `blocking: true`, finding counts, findings body, and the per-file `status` map (`pass`/`fail` for every reviewed file — full runs have no `carried`; the map is the failure-recovery scope input, derived mechanically by `gate-finalize` from the accepted packet verdicts) |
| `revalidate@{target}` / `reverify@{unit}` / `rereview@{unit}` delta PASS | Rewrite the gate's cache with `mode: full`, `basis: delta`: new evidence for re-run judgments, original evidence for carried-over judgments (see §Delta Runs) |
| Delta run FAIL (P0/P1 findings) | Write a failure record — rewrite the gate's cache with `result: fail`, `blocking: true`, severity counts, findings body, `basis: delta`, and the per-check `status` map. Promote must not proceed (see §Delta Runs → Failure recovery) |
| Any targeted run (`:check-{n}` / `:{keyword}`) | PASS with only P2/P3 findings → report findings and leave cache state unchanged. P0/P1 findings → run `gate-invalidate`: delete a pass cache or persist the key on a failure record, and invalidate a matching open run. Targeted runs never publish a complete cache result |

### Cache semantics

- A cache exists only when a complete-coverage run (full or delta) completed. `mode: full` is the only value ever written; `basis` records whether the cache came from a full run (`full` or absent), an incremental one from a pass baseline (`delta`), or an incremental recovery from a failure record (`repair`).
- A targeted PASS never writes or downgrades an existing cache. A targeted P0/P1 records blocking invalidation through `gate-invalidate`: a pass cache is deleted; a failure record is kept with the contradicted key persisted for repair.
- Any FAIL at full granularity deletes the candidate cache for validate (full-run failure — trust establishment failed; the spec is the upstream root, see §Failure handling by gate role in `framework/validation_cache.md`); verify and review full FAILs write a failure record. Any FAIL at delta granularity writes a failure record for all three gates. P0/P1 at any granularity means promote must not proceed.

## Relationship with Promote

- `specflowctl promote --unit <name>` requires fresh caches for validate, verify, review, and appendix coverage. Every complete-result cache has `mode: full` by construction (targeted runs never publish one; delta runs keep `mode: full` and record `basis: delta`/`repair` — the promote gate checks the same evidence for all three).
- The validate cache must have `result: pass`; the verify cache must have `result: pass` (P2/P3 pending items are carried by the severity counts — non-blocking findings pass the promote gate); the review cache must be non-blocking.
- A failure record (`result: fail` + `blocking: true`) in any gate is rejected as BLOCKED — the findings must be resolved first. A `result: fail` cache without consistent blocking declarations is an invalid write and fails closed.
- No cache at all → promote rejected.

## State Transition Disclosure

| Cache state | Disclosure |
|-------------|-----------|
| validate cache fresh | "Validate passed all checks" |
| verify cache fresh | "Verify passed all content" |
| verify cache fresh (result: pass, blocking: false, p2_count/p3_count > 0) | "Verify passed with P2/P3 pending findings — promote may proceed" |
| review cache fresh | "Review passed — no P0 or P1 findings" |
| Failure record (blocking) | "Gate found P0/P1 findings — resolve them, then the delta re-run recovers incrementally" |
| No cache / stale | "Cache does not exist or is expired, needs re-checking" |

After a targeted run, the agent offers:
- "Run `verify@user_auth` for complete verification"
- "Or specify a section to verify by keyword"

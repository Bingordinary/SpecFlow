# Verification Scope

## Problem

`validate`, `verify`, and `review` operate on a spec unit against the relevant codebase or document set. The older scoped/full dual-mode design used git working-directory changes to pick a subset. Practice showed that subset results are structurally unusable: the promote gate only accepts results from a complete run, so a scoped result always had to be re-run in full before promote — the scoped pass was wasted work. The subset also did not match what the user actually cares about: git diff reflects "what was recently changed", not "what the user is worried about".

The one reason for scoped mode was context saturation on large codebases. That problem is solved structurally by sub-agent batching (see Full Verify below) — the main agent distributes work to read-only sub-agents instead of shrinking the checked surface.

## Solution

There are two complete-coverage run modes: **full** (`validate@{target}`, `verify@{unit}`, `review@{unit}`) and **delta** (`revalidate@{target}`, `reverify@{unit}`, `rereview@{unit}`). A full run checks everything in scope. A delta run re-checks the judgments whose dependency evidence went stale and carries the rest over — the result is a complete check either way (see §Delta Runs).

Targeted checking exists only through explicit user choice: `:check-{n}` and `:{keyword}`. The user declares the focus, not git diff. Targeted runs are for iterative feedback — they never write a cache, so they can never satisfy the promote gate.

**Cache invariant:** only a complete-coverage run (full or delta) writes a cache. A cache exists means a complete check passed (or, for review, a complete review completed). Targeted runs report findings and write nothing.

**Layer roles:** the verification loop — caches, promote eligibility, and delta re-runs — belongs to the candidate layer. The stable layer has no gate; the three full commands run against a stable-only target as **confirmation checks** of the stable content's continuing relationship with the outside world (see §Stable-only Targets). They write a `target: stable` cache consumed by `fresh@stable` and never edit anything — any change to consensus content goes through `fork` (see `framework/concepts.md` §4). Delta re-runs restore a stale confirmation cache when a usable pass baseline exists (see §Delta Runs → Layer applicability).

## Principles

1. **Full by default, complete always** — every command without a `:` suffix runs the complete check; delta runs (`re*`) also produce a complete check by re-checking the stale part and carrying the rest over. No git-awareness, no subset selection.
2. **Targeted only on explicit user choice** — `:check-{n}` (validate) and `:{keyword}` (all three commands) scope the run to a user-declared focus.
3. **Targeted results never satisfy promote** — targeted runs do not write caches. Only a complete-coverage run's cache passes the promote gate.
4. **Delta only on explicit user choice** — `revalidate@{target}` / `reverify@{unit}` / `rereview@{unit}` re-run only the judgments whose evidence went stale (plus cross-check — unit targets) and write a cache with `basis: delta`. The incremental scope is derived from the stale cache evidence and reported to the user explicitly (see §Delta Runs).
5. **Delta restores both layers** — the incremental re-run restores promote eligibility for candidates and a stale confirmation state for stable-only targets (whose confirmation cache exists with `result: pass`, review: `blocking: false`); a failure record (BLOCKED cache) is restored by the failure-recovery delta run (`basis: repair`). A delta run against a stable-only target without a usable baseline reports "No usable confirmation baseline. Run the full command" (see §Delta Runs → Layer applicability).
6. **Keyword means "what the user wants to look at"** — each command resolves the keyword inside its own target domain (see Keyword Resolution).
7. **No fixed item definition** — the framework does not define what an "item" is. The agent reads the spec structure dynamically.

## Syntax

### Validate

| User says | What agent does |
|-----------|-----------------|
| `validate@{target}` | Full: all 8 checks + cross-check (unit only — rules have no cross-check). Candidate target: writes the validate cache (`mode: full`, promote gate). Stable-only target (no candidate file): the same 8 checks against the stable content and its current dependencies and rules — writes the cache with `target: stable` (confirmation state consumed by `fresh@stable`); on FAIL writes a failure record and recommends forking (see §Stable-only Targets). Writes via `specflowctl cache-write` (`specflow/tooling/bin/specflowctl-<os>-<arch>`, `<tooling-root>` is `specflow/tooling`) — see `framework/validation_cache.md` §Write Rules (Tooled writes). Main agent MUST follow the report with `cache-write`; otherwise `fresh` stays `MISSING` (or `BLOCKED` for a failure record) and `promote` is rejected — see `framework/unit_validate_checklist.md` §Completion — Persist Gate Cache. |
| `validate@{target}:check-{n}` | Targeted: single check `{n}` only. User explicitly chooses focus. Does not write a cache. Targeted runs intentionally do NOT write a cache; completion is the report alone (see `framework/unit_validate_checklist.md` §Completion — Persist Gate Cache). |
| `validate@{target}:{keyword}` | Targeted: matches keyword to a check name (e.g., "design" → Check 2, "scope" → Check 3). Does not write a cache. Targeted runs intentionally do NOT write a cache; completion is the report alone (see `framework/unit_validate_checklist.md` §Completion — Persist Gate Cache). |

### Verify

| User says | What agent does |
|-----------|-----------------|
| `verify@{unit}` | Full: verify all spec content (all 7 steps, batch by spec structure) + cross-check. Candidate target: writes the verify cache (`mode: full`, promote gate); on FAIL (P0/P1) writes a failure record with the per-item `status` map — the `reverify@{unit}` failure-recovery baseline (see §Failure handling by gate role in `framework/validation_cache.md`). Stable-only target (no candidate file): verify the stable spec against the code (drift confirmation) — writes the cache with `target: stable` (VERIFIED state consumed by `fresh@stable`); on MISMATCH writes a failure record, reports the drift, and recommends forking (see §Stable-only Targets). Writes via `specflowctl cache-write` (`specflow/tooling/bin/specflowctl-<os>-<arch>`, `<tooling-root>` is `specflow/tooling`) — see `framework/validation_cache.md` §Write Rules (Tooled writes). Main agent MUST follow the report with `cache-write`; otherwise `fresh` stays `MISSING` (or `BLOCKED` for a failure record) and `promote` is rejected — see `framework/unit_verify_checklist.md` §Completion — Persist Gate Cache. |
| `verify@{unit}:{keyword}` | Targeted: matches keyword to spec content (section title, feature name, API path, etc.) → verify that content. Does not write a cache. Targeted runs intentionally do NOT write a cache; completion is the report alone (see `framework/unit_verify_checklist.md` §Completion — Persist Gate Cache). |

### Spec Review

| User says | What agent does |
|-----------|-----------------|
| `review@{unit}` | Full: read all files referenced in the candidate spec's `affects.files` and `implementation_surface` across all acceptance items → review those files with spec context. Candidate target: writes the review cache (promote gate). Stable-only target (no candidate file): review with the stable spec as design context (code-quality confirmation) — writes the cache with `target: stable`; implementation-class defects may be fixed in code and re-reviewed, design-class defects lead to forking (see §Stable-only Targets). Writes via `specflowctl cache-write` (`specflow/tooling/bin/specflowctl-<os>-<arch>`, `<tooling-root>` is `specflow/tooling`) — see `framework/validation_cache.md` §Write Rules (Tooled writes). Main agent MUST follow the report with `cache-write`; otherwise `fresh` stays `MISSING` (or `BLOCKED` for a failure record) and `promote` is rejected — see `framework/spec_review_checklist.md` §Completion — Persist Gate Cache. |
| `review@{unit}:{keyword}` | Targeted: matches keyword to a file name in `affects.files` or `implementation_surface` → review that file. Does not write a cache. Targeted runs intentionally do NOT write a cache; completion is the report alone (see `framework/spec_review_checklist.md` §Completion — Persist Gate Cache). |

### Rule (validate only, verify removed)

| User says | What agent does |
|-----------|-----------------|
| `validate@{rule}` | Full: all 8 checks (7 metadata + 1 body quality). Writes the validate cache (`mode: full`, promote gate) via `specflowctl cache-write` — see `framework/validation_cache.md` §Write Rules. Main agent MUST follow the report with `cache-write`; otherwise `fresh` stays `MISSING` (or `BLOCKED` for a failure record) and `promote` is rejected — see `framework/rule_validate_checklist.md` §Completion — Persist Gate Cache. |
| `validate@{rule}:check-{n}` | Targeted: single check `{n}` only. User explicitly chooses focus. Does not write a cache. Targeted runs intentionally do NOT write a cache; completion is the report alone (see `framework/rule_validate_checklist.md` §Completion — Persist Gate Cache). |
| `validate@{rule}:{keyword}` | Targeted: matches keyword to a check name. Does not write a cache. Targeted runs intentionally do NOT write a cache; completion is the report alone (see `framework/rule_validate_checklist.md` §Completion — Persist Gate Cache). |

> `verify` on a Rule target has been removed. If the user says `verify@{rule}`, report: "Rule verify has been removed. Run `validate@{rule}` instead." See `framework/concepts.md` for context.

### Delta re-runs (re* instruction family)

| User says | What agent does |
|-----------|-----------------|
| `revalidate@{target}` | Delta: re-run only the checks whose dependency evidence went stale + cross-check (unit targets), carry the rest over; a failure-record baseline re-runs the failed checks instead (see §Delta Runs → Failure recovery). Writes a cache with `mode: full`, `basis: delta` (`repair` from a failure record); delta FAIL writes a failure record. Writes via `specflowctl cache-write` (`specflow/tooling/bin/specflowctl-<os>-<arch>`, `<tooling-root>` is `specflow/tooling`) — see `framework/validation_cache.md` §Write Rules (Tooled writes). Candidate targets, plus stable-only targets with a usable baseline (see §Delta Runs → Layer applicability). Preconditions and scope rules in §Delta Runs. Main agent MUST follow the report with `cache-write --basis delta/repair`; otherwise `fresh` stays `MISSING`/`BLOCKED` and `promote` is rejected — see the target-appropriate checklist §Completion — Persist Gate Cache. |
| `reverify@{unit}` | Delta: re-verify only the spec content whose evidence went stale + cross-check, carry the rest over; a failure-record baseline re-runs the failed judgments instead. Writes a cache with `mode: full`, `basis: delta` (`repair` from a failure record); delta FAIL writes a failure record. Writes via `specflowctl cache-write` (`specflow/tooling/bin/specflowctl-<os>-<arch>`, `<tooling-root>` is `specflow/tooling`) — see `framework/validation_cache.md` §Write Rules (Tooled writes). Candidate targets, plus stable-only targets with a usable baseline (rule verify has been removed). Main agent MUST follow the report with `cache-write --basis delta/repair`; otherwise `fresh` stays `MISSING`/`BLOCKED` and `promote` is rejected — see `framework/unit_verify_checklist.md` §Completion — Persist Gate Cache. |
| `rereview@{unit}` | Delta: re-review only the files whose evidence went stale + cross-check, carry the rest over; a blocking baseline re-runs the failed dimensions instead. Writes a cache with `mode: full`, `basis: delta` (`repair` from a blocking cache); delta FAIL writes the blocking cache extended with the per-check status map. Writes via `specflowctl cache-write` (`specflow/tooling/bin/specflowctl-<os>-<arch>`, `<tooling-root>` is `specflow/tooling`) — see `framework/validation_cache.md` §Write Rules (Tooled writes). Candidate targets, plus stable-only targets with a usable baseline. Main agent MUST follow the report with `cache-write --basis delta/repair`; otherwise `fresh` stays `MISSING`/`BLOCKED` and `promote` is rejected — see `framework/spec_review_checklist.md` §Completion — Persist Gate Cache. |

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

## Sub-agent Prompt Assembly

The sub-agent prompt assembly structure is shared by the three quality-gate commands. The main agent assembles the prompt from the target's files and the command's own checklist, following this fixed structure. Command-specific values are listed per field below.

The three commands use two execution shapes:

1. **Independent single-executor sub-agent (validate)** — `validate@{unit}` / `validate@{rule}` (full and delta runs) execute the checklist in one read-only sub-agent session. There is no batching: the sub-agent runs the whole checklist itself and returns one report. The purpose is independence, not context economy — the sub-agent does not hold the context in which the spec was written, so its verdicts are not self-approval (see `framework/spec_flow_review.md` §2.3: promote's quality gates are independent sessions, not self-approval).
2. **Batched parallel sub-agents (verify, review)** — full verify and review runs split work into batches across parallel read-only sub-agents. Batching exists to avoid context saturation on code surfaces; batch splitting is an internal optimization and the result is independent of it.

A sub-agent prompt is a **mission package for a zero-context worker**: the sub-agent has no conversation history and no framework knowledge beyond this prompt and the files it is told to read. The prompt must therefore answer five questions without requiring inference — who am I, what am I doing, why, what counts as done, and who consumes my output. The mandatory fields below guarantee this.

**Mandatory fields:**

1. **Role line** —
   - validate: "read-only validation sub-agent for {unit|rule} {name}", followed by "You are an independent read-only session: you do not hold the context in which this spec was written, and the main agent does not re-litigate your verdicts — it collects your output verbatim into the final report."
   - verify: "read-only detection sub-agent for verify batch {n} of {N}", followed by "You are one of {N} parallel read-only workers; the main agent collects all workers' results verbatim into the final report."
   - review: "read-only review sub-agent for review batch {n} of {N}", followed by the same parallel-worker sentence.
2. **Mission statement** — one sentence stating the deliverable, fixed form:
   - validate: "Validate {target} per the protocol: execute each check in Check scope, report PASS / WARNING / FAIL per check with a reason (FAIL reasons identify the contradicting information sources), and report findings with P0/P1 severity and resolution type."
   - verify: "Verify each item in Batch scope: decide ALIGNED / MISMATCH / CANNOT_DETERMINE per the protocol, with deterministic evidence for every claim."
   - review: "Review each file in Batch scope per the protocol and report findings with P0-P3 severity."
3. **Spec source** — main spec path + version, plus the appendix directory (all non-exempt, non-retired appendices are part of the spec, per the checklist prerequisites)
4. **Check / Batch scope** —
   - validate full: "all 8 checks + cross-check" (unit) / "all 8 checks" (rule — rules have no cross-check); validate delta: re-run checks {list} + cross-check (unit; rules have no cross-check), carried-over checks {list} declared (see §Delta Runs); validate targeted (`:check-{n}` / `:{keyword}`): executed directly by the main agent, no sub-agent is launched (see §Targeted Runs)
   - verify: section titles and acceptance item ids as anchors (not line numbers — the spec is a moving target)
   - review: the file list itself is the anchor
   - **Delta runs (validate/verify/review):** the prompt's Check/Batch scope lists the re-run checks only; the sub-agent reports `Dependency scope` for the re-run checks it actually executed — carried-over checks are not re-executed and get no new scope declaration (their evidence stays in the cache unchanged)
5. **Read surface** — the files the sub-agent may need:
   - validate (unit): the spec and its referenced dependency files (`unit_refs`, `rule_refs`, `affects.files` document targets); implementation code is not read
   - validate (rule): the candidate rule file, its stable sibling if present (Check 4), and the unit spec files under `docs/specs/units/` (Check 5 existence check and Check 7 consumer discovery)
   - verify / review: from `implementation_surface` and `affects.files`
6. **Protocol reference** — the command's own checklist, as the only protocol source: validate unit → `framework/unit_validate_checklist.md` (all 8 checks, cross-check included); validate rule → `framework/rule_validate_checklist.md` (all 8 checks, no cross-check); verify → `framework/unit_verify_checklist.md` Steps 1-6 (Step 2's Part A/B sub-checks included); review → `framework/spec_review_checklist.md`
7. **Context** — the background the sub-agent needs to orient itself, four fixed sentences: which command and target this run serves ("You are part of the `validate@{unit}` full run"); execution-shape declaration (validate: "You are an independent read-only session — your verdicts are not self-approval, and the main agent collects them verbatim"; verify: "Batch boundaries are an internal optimization — your result is the same whether an item is verified alone or in a group"; review: "Batch boundaries are an internal optimization — your result is the same whether a file is reviewed alone or in a group"); output consumption ("Your output is collected verbatim by the main agent into the final report; follow the protocol's output format exactly"); unit orientation ("{unit} is {one-sentence description}; candidate version {version}"). The one-sentence unit description is distilled from the spec's goal/responsibility sections (spec path from `specflowctl next` output); the version comes from the spec frontmatter.
8. **Glossary** — one line per term used in this prompt or in the protocol steps this sub-agent executes. Each line: term — one-sentence definition — source reference. The canonical glossary below is the full set for the three quality-gate commands; the main agent includes every term that appears in the assembled prompt and no others. Scenarios outside the three commands (e.g. the `spec_flow_issues` triage prompt) define their own glossary in their protocol file (`framework/operations/issues.md` Step 3).
9. **Permissions** — verbatim: "You may read files, search text by pattern, glob for files, and run read-only git queries. You must NOT modify any file, run any command that changes state, or launch further sub-agents."

   **Assembly rule — cache-write tool path:** When the main agent assembles the sub-agent prompt, it MUST resolve `specflow/tooling/bin/specflowctl-<os>-<arch>` to the concrete binary for the current execution environment (e.g. `darwin-arm64`, `linux-amd64`, `windows-amd64.exe` — list `specflow/tooling/bin/specflowctl-*` to find the installed binary; `uname -s`/`uname -m` mapping is the fallback). The placeholder `<os>-<arch>` MUST NOT appear in the assembled prompt. The sub-agent MUST NOT attempt to infer or construct the tool path.

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
| Dependency scope | The report's per-check dependency declaration: one line per check stating the file and the section-region headings (or line ranges, or `all`) that check's judgment depended on, used by the main agent when writing the cache's per-check `checks` mapping | `framework/unit_verify_checklist.md` §Output Format |
| section region | A content region of a markdown file located by heading: the frontmatter region (file head through the line before the first `##` heading, heading `""`) or one `##` heading section (heading line through the line before the next `##` heading; deeper headings belong to their `##` section). Declared as `region:section:<heading>:<cid>` | `framework/validation_cache.md` §Structural Region Dependencies |
| structural region | A content region located by structure rather than line numbers: the acceptance item set (`region:acceptance_items:<cid>`, from the `acceptance_item_set:` marker to the next top-level heading) or a section region; edits outside the region do not stale a declaration on it | `framework/validation_cache.md` §Structural Region Dependencies |
| Part A / Part B | The two parts of the test design sub-check: coverage completeness / test meaningfulness | `framework/unit_verify_checklist.md` Step 2 |
| B1-B6 | The six Part B checks: mock density, assertion authenticity, tautological assertions, all-happy-path, mock-through, test naming | `framework/unit_verify_checklist.md` Step 2 |
| stub | A placeholder or debt marker in implementation code (Step 6 grep findings; RELEVANT hits are MISMATCH) | `framework/unit_verify_checklist.md` Step 6 |
| surplus | Code structure with no spec correspondence (reported as MISMATCH type: surplus) | `framework/unit_verify_checklist.md` Step 5 |
| structural / acceptance / scope | The remaining MISMATCH type values: structural (Step 1 declaration mismatch), acceptance (Step 2 pass_condition mismatch), scope (Step 3 affects declaration mismatch) | `framework/unit_verify_checklist.md` Steps 1-3 |
| severity (verify / review) | The P0-P3 grade of a finding; blocking semantics per the shared severity policy | `framework/severity_policy.md` §4 |
| fact anchor | A reproducible repository fact that proves a P3 discrepancy by identifying its governing or comparison reference, violating location, and relationship | `framework/spec_review_checklist.md` §5 P3 Reportability Gate |
| P3 reportability gate | The review gate that permits a P3 finding only when its discrepancy is objective, local, low-impact, and supported by a reproducible fact anchor | `framework/spec_review_checklist.md` §5 P3 Reportability Gate |
| §9 severity confirmation | The severity consistency check the main agent runs before cache write or final report, recording confirmed or adjusted per finding | `framework/severity_policy.md` §9 |
| cross-check | The final consistency check over all content after individual checks pass | `framework/verification_scope.md` §Cross-check |
| check 1-8 (validate) | The eight validate checks: structural integrity, design soundness, scope integrity, evidence-driven vs design-driven consistency, acceptance coverage & correctness, affects-source validity, cross-unit consistency, constraint alignment | `framework/unit_validate_checklist.md` |
| exempt / retired appendix | Appendix statuses skipped when assembling the spec union (`status: exempt` / `status: retired` files are not read) | `framework/unit_validate_checklist.md` §Prerequisite |
| resolution type | Each finding's fix classification: `actionable` (concrete repair without user judgment) or `needs_decision` (requires user input) | `framework/unit_validate_checklist.md` §Execution Rules |
| advisory finding | validate's non-blocking items: Check 1 step 7 / step 13 hygiene WARNING and Check 2 Step 4 taste-level P2/P3, presented on the check line's reason, counted separately, never blocking | `framework/unit_validate_checklist.md` §Counting rules |
| extraction artifact | The falsifiable evidence carried by uncovered-domain (5a) and uncarried-contract (5h) FAIL findings, with three parts: the quoted source declaring the behavior domain or contract statement, the granularity/external-visibility judgment, and the absence claim over the covered surface (the item set union / the carriers) | `framework/unit_validate_checklist.md` 5a step 8 / 5h step 4 |
| external-visibility boundary | The §4 judgment separating contract statements (externally-observable behavior a caller or dependent unit must read to use the unit safely) from internal design expression (internal field names, internal field layouts, internal timing incl. retry/backoff values, internal data structures and their operations, internal function behavior, configuration layout); content on the internal side creates no coverage or carrier obligation | `framework/spec_writing_guide.md` §4 |
| P0 / P1 severity (validate) | The only severities validate grades: P1 is the contract-decided default for FAIL checks; P0 requires §9 confirmation | `framework/unit_validate_checklist.md` §Severity check |
| Dimension 8 (module_boundaries / responsibility_organization / dependency_clarity / abstraction_level / extension_landing_points / engineering_patterns) | The architectural design quality assessment of the reviewed code surface: per-batch lines reporting module boundaries, responsibility organization, dependency clarity, abstraction levels, extension landing points, and engineering patterns, each with an assessment and basis | `framework/spec_review_checklist.md` §4 (Dimension 8) / §Body format |
| spec_context | The finding's attached relevant design context from the spec, helping the user understand the code-design relationship | `framework/spec_review_checklist.md` §Findings section / §6 |
| recommendation | The finding's fix suggestion | `framework/spec_review_checklist.md` §Findings section / §6 |

**File-list baseline:** the main agent runs `specflowctl next --unit <name>` and uses its output (spec file, appendices, implementation surface, affects files, acceptance item ids) as the mechanical baseline for fields 3-5. Test files are collected by globbing `*_test.go` next to each implementation file — never by guessing. For validate, the same command output supplies the spec, the appendix directory, and the dependency targets (`unit_refs` / `rule_refs`) that make up the read surface. For `validate@{rule}` — `specflowctl next` supports unit targets only — the mechanical baseline is the command target file `docs/specs/rules/candidate/{rule_id}.md`, its stable sibling `docs/specs/rules/stable/{rule_id}.md` if present (Check 4), and the unit spec files globbed under `docs/specs/units/` (Checks 5/7).

An `implementation_surface` value of `<pending>` produces no file-list entry — the placeholder declares an unknown implementation surface; its judgment belongs to verify Step 6 (MISMATCH), it is never collected as a file.

**Prohibitions:**

1. No inline restatement of protocol rules (severity tables, evidence thresholds, Part B check definitions) — the sub-agent reads the checklist itself. This prohibition covers protocol rules only: the context declarations above (fields 1-9) are mandatory and are not restatements — they locate the protocol, they do not replace it. When any prompt text conflicts with the protocol, the protocol wins and the sub-agent reports the conflict.
2. No severity assignment outside the command's own rules — validate sub-agents grade findings P0/P1 with a resolution type per `framework/unit_validate_checklist.md` (P1 is the contract-decided default; P0 requires the §9 confirmation record — see its Severity check section); verify detection sub-agents report MISMATCH type (structural / acceptance / scope / stub / surplus) only — severity assignment is Step 7's job (the analysis sub-agent grades per the Step 7 output contract, the main agent confirms per §9), and stub type determination (RELEVANT/IRRELEVANT) is Step 6 detection output, not analysis classification (see `framework/unit_verify_checklist.md` Step 6 and the Step 7 full-mode note); review sub-agents grade findings P0-P3 per the review checklist's own gate rules.
3. No behavior summary presented as normative — the spec is the only normative source; a prompt summary that conflicts with the spec is reported as a prompt/spec discrepancy, spec wins
4. No cache file writes by sub-agents — sub-agents MUST NOT write cache files and MUST only report findings plus `Dependency scope:` lines; the main agent assembles the cache via `specflowctl cache-write` (`specflow/tooling/bin/specflowctl-<os>-<arch>`, `<tooling-root>` is `specflow/tooling`, resolved to the concrete binary by the main agent — see `framework/validation_cache.md` §Write Rules → Tooled writes). The declaration schema has no `hash`/`deps` fields — a transcribed CID cannot enter a cache file. The sub-agent MUST NOT attempt to infer or construct the tool path.

**Required output fields:**

- validate per check: `{n}. {check name}: PASS | WARNING | FAIL — reason`; FAIL reasons identify the contradicting information sources per the checklist's Execution Rules; findings use the unified format `[{P0|P1}] {location} — {issue} (actionable | needs_decision)`; cross-check line for unit full runs
- validate extraction evidence: sub-check 5a uncovered-domain and sub-check 5h uncarried-contract FAIL findings must include the extraction artifact defined in the checklist (5a step 8 / 5h step 4) — the quoted source declaring the behavior domain or contract statement, the granularity/external-visibility judgment, and the absence claim over the covered surface (the item set union / the carriers) — so the main agent can re-verify the claim before presenting the finding
- verify per item: `{item.id}: ALIGNED | MISMATCH (type) | CANNOT_DETERMINE` with code references — the type is the detection verdict (structural / acceptance / scope / stub / surplus); detection sub-agents do not assign severity — the Step 7 analysis sub-agents grade it per the Step 7 output contract and the main agent confirms it per §9 before the final report; review: findings per the review checklist's output format, with a `fact_anchor` on every P3 finding
- review batch assessment: each review batch reports the per-batch Dimension 8 assessment lines (`module_boundaries`, `responsibility_organization`, `dependency_clarity`, `abstraction_level`, `extension_landing_points`, `engineering_patterns`, each with an assessment and basis) for the files in its batch, per `framework/spec_review_checklist.md` §Body format; the main agent aggregates the per-batch assessments into the final report's Architecture assessment block
- `Dependency scope:` one line per check the run executed — `{check key}: {file}: {declaration}` where `{check key}` is the command's check identifier (validate: `check-{n}`; verify: the acceptance item id; review: the batch/file dimension), `{file}` is the file the judgment read, and `{declaration}` is the section-region heading text the check's judgment read (e.g. `Description`, `Testability / Acceptance Criteria`, or the frontmatter region as `frontmatter`), 1-based closed line ranges, or `all` for whole-file judgments. This is the sub-agent's only evidence declaration — it MUST NOT write `hash`/`deps` values. The main agent maps the declaration onto the cache's per-check `checks` mapping (`--section` for section headings, `--ranges` for line ranges; see the unified report skeleton and `framework/validation_cache.md` §Format) and the tooling computes the CIDs via `specflowctl cache-write` (see `framework/validation_cache.md` §Write Rules → Tooled writes). Delta runs report the scope of the re-run checks only (see Check scope above)
- failure path: "Validation could not complete — {reason}" (verify: "Verification could not complete — {reason}"; review: "Review could not complete — {reason}")

**Assembly example (verify)** — a complete assembled verify prompt. `{unit}`/paths are placeholders the main agent fills in; the glossary shows only the terms used in this prompt:

```
You are a read-only detection sub-agent for verify batch 2 of 4. You are one of 4
parallel read-only workers; the main agent collects all workers' results verbatim
into the final report.

Mission: Verify each item in Batch scope: decide ALIGNED / MISMATCH /
CANNOT_DETERMINE per the protocol, with deterministic evidence for every claim.

Spec source: docs/specs/units/candidate/unit_auth.md (candidate, version 0.2.1);
appendices: docs/specs/units/candidate/appendix/unit_auth_*.md (non-exempt,
non-retired only).

Batch scope: AUTH-AC-003, AUTH-AC-004 (login section).

Implementation surface: src/api/login.go, src/api/token.go, src/store/session.go.

Protocol: framework/unit_verify_checklist.md Steps 1-6 (including Step 2's
Part A/B sub-checks). This is the only protocol source — follow it exactly.

Context: You are part of the `verify@auth` full run. Batch boundaries are an
internal optimization — your result is the same whether an item is verified
alone or in a group. Your output is collected verbatim by the main agent into
the final report; follow the protocol's output format exactly. auth is the user
authentication unit; candidate version 0.2.1.

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
- Dependency scope — per-file line ranges your judgment depended on
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
  analysis sub-agents assign it and the main agent confirms it per §9; plus
  Part A and Part B findings per acceptance item
- Dependency scope: one line per item aligned — {item.id}: {file}: {section
  heading | line ranges | "all"} (the sections/regions of the spec the item's
  judgment depended on)
- If you could not complete: "Verification could not complete — {reason}"
```

**Assembly example (validate)** — a complete assembled validate prompt for a unit full run. `{unit}`/paths are placeholders the main agent fills in; the glossary shows only the terms used in this prompt:

```
You are a read-only validation sub-agent for unit payment. You are an independent
read-only session: you do not hold the context in which this spec was written,
and the main agent does not re-litigate your verdicts — it collects your output
verbatim into the final report.

Mission: Validate unit payment per the protocol: execute each check in Check
scope, report PASS / WARNING / FAIL per check with a reason (FAIL reasons identify
the contradicting information sources), and report findings with P0/P1 severity
and resolution type.

Spec source: docs/specs/units/candidate/unit_payment.md (candidate, version 1.3.0);
appendices: docs/specs/units/candidate/appendix/unit_payment_*.md (non-exempt,
non-retired only).

Check scope: all 8 checks + cross-check.

Read surface: the spec above plus its referenced dependency files (unit_refs,
rule_refs, affects.files document targets). Implementation code is not read.

Protocol: framework/unit_validate_checklist.md — all 8 checks plus the cross-check
section. This is the only protocol source — follow it exactly.

Context: You are part of the `validate@payment` full run. You are an independent
read-only session — your verdicts are not self-approval, and the main agent
collects them verbatim. Your output is collected verbatim by the main agent into
the final report; follow the protocol's output format exactly. payment is the
payment processing unit; candidate version 1.3.0.

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
- Cross-check: N/N PASS — per-check results
- Failed checks: N | Advisory findings: K
- Dependency scope: one line per check executed — check-{n}: {file}: {section
  heading | line ranges | "all"} (the sections/regions of the spec the check's
  judgment depended on; for the unit's own main spec, name the section-region
  headings, e.g. "Description" or "Testability / Acceptance Criteria"; the
  frontmatter region is "frontmatter"; cross-check reports its scope like any
  other line, with check key `cross` (see `framework/validation_cache.md`
  §Format → Per-check evidence)
- If you could not complete: "Validation could not complete — {reason}"
```

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

The delta scope is **derived by mechanism**: the cache's per-check evidence (`checks`, see `framework/validation_cache.md` §Format) maps every check to the regions it declared; the stale regions are matched against that map to compute the affected check set. The agent does not guess which judgments a content change affects — it reports the derived scope, executes it, and covers the rest by carry-over. The map covers per-check declarations only: entries without a `checks` mapping (logical references, contract files, whole-file declarations, declare-heavy extras) have no check association, so their staleness is reported as **unclaimed** and mapped to checks by the rules below (§Incremental scope step 1) — never silently carried over.

### When delta applies

A delta run has two baseline forms — a **pass baseline** (the stale-cache recovery) and a **failure baseline** (the failure-record recovery, §Failure recovery below):

1. **Pass baseline** — a cache exists for the gate, with `mode: full` and `result: pass` (review: `blocking: false`), and it has at least one stale source — a `files` entry whose declared dependency CID is no longer present. If nothing is stale, report "cache is fresh — no incremental re-run needed" and stop. A blocked review cache (P0/P1 findings) is not this baseline — it is the failure baseline instead.
2. **Failure baseline** — a cache exists for the gate as a **failure record** (`result: fail` + `blocking: true`; for validate: written by a delta re-run's FAIL; for verify: written by a delta re-run's FAIL or a candidate full-run FAIL; for review: the blocking cache). The recovery re-runs the failed judgments plus the newly affected ones — no stale source is required, the failed judgments themselves are the re-run reason.
3. A MISSING cache has no baseline to carry over from — run the full command instead.

**Degradation is a derived conclusion, not a rule:** the affected check set is the set of checks whose declared dependencies went stale (plus the cross-check, which is always re-run). When that set covers **every** check the cache declares, the incremental scope is the whole run — report this to the user ("the stale sources affect every check — an incremental re-run is a full re-run") and ask whether to proceed or run the full command instead. This is not a special case of own-spec staleness: editing one section of your own spec stales only the checks that declared that section, and the delta re-runs exactly those. The same degradation conclusion applies to a failure-record recovery when the failed judgments plus the newly affected ones cover every declared check.

### Incremental scope (mechanism-derived)

1. Run the scope derivation over the stale cache: `specflowctl fresh@{target}` reports the stale files, and for every STALE gate its detail output appends a `DELTA SCOPE (<gate>)` section — the mechanism-derived scope (the affected check keys from the cache's per-check `checks` mapping, the unclaimed entries, the stale-dep count, and the degradation state). The affected check set = {checks whose declared deps went stale} ∪ {cross-check}. Stale dependencies that no check declared — logical-reference entries (`unit:{name}` / `unit:{name}:appendix:{file}` / `rule:{id}`), contract files, whole-file declarations, and declare-heavy extras in the file-level union — are **unclaimed**: the derivation reports them as such (per entry, e.g. `unit:auth`) and the agent maps them to the checks that read them by the command's fixed association, never by guessing and never by silent carry-over:
   - unit `validate`: a `unit:` entry → Check 7 (cross-unit); a `rule:` entry → Check 8 (constraint alignment).
   - rule `validate`: a `unit:` entry → the consumer-discovery checks (Check 5/7).
   - `verify`: an unclaimed code file or dependency entry → the acceptance items whose judgment read that file (semantic derivation — the item→file association is dynamic, see `framework/unit_verify_checklist.md`).
   - `review`: an unclaimed code file or dependency entry → the review dimensions that assessed it (semantic derivation).
   - When no fixed association exists (own-spec whole-file declarations), re-read the changed content and map it semantically; include the affected judgments conservatively (declare-heavy, step 4).
   When the cache has no per-check evidence (written before `checks` support — `fresh@` reports "no per-check evidence"), derive the scope from the stale file sources instead: re-read the spec (and the code structure for verify/review) to map the changed content to the affected checks — the semantic derivation kept as the fallback for legacy caches (see `framework/unit_validate_checklist.md` §Delta re-run step 2).
2. **Derivation vs `fresh@`:** the derivation judges per-check deps while `specflowctl fresh@{target}` judges file-level deps — the two can disagree (a whole-file chunk staleness is invisible to the check-level map). When they disagree, `fresh@` is authoritative: treat the file as a stale source, re-check its unclaimed deps, and map them per step 1.
3. For files whose hash changed but declared deps are unchanged: quickly re-scan them for semantic coupling with any judgment — if coupling exists, include the affected judgments in the scope (this operationalizes the informational note in `framework/validation_cache.md` §Staleness Detection).
4. The scope must always include the cross-check (unit targets — rules have no cross-check) — it is a holistic judgment over all content, and changed shared content can change it.
5. Report the scope explicitly before executing: the re-run checks with their stale source and reason (including the unclaimed entries and the checks they mapped to), and the carried-over checks with the statement that their dependency evidence is unchanged. Declare conservatively — when unsure whether a judgment is affected, include it (declare-heavy, same principle as dependency declaration).

### Execution

- Re-run only the scoped judgments. Carried-over judgments are not re-executed — their previous results stand on unchanged evidence.
- Any P0/P1 finding during the re-run writes a **failure record**: validate/verify/review rewrite the cache with `result: fail`, `blocking: true`, severity counts, a findings body, `basis: delta`, and the per-check `status` map (`fail`/`pass` for the re-run judgments, `carried` for the carried-over ones). Promote must not proceed. (This replaces the old delete-on-delta-FAIL behavior — the record is the failure-recovery baseline.)
- On PASS, rewrite the cache with `mode: full`, `basis: delta`, a fresh `timestamp`, and a complete `files` list: re-run judgments replace their entries with new evidence (`specflowctl gate-evidence` on the files they read — including the per-check `checks` breakdown for the re-run checks); carried-over judgments keep their entries with the original hash, deps, and checks — their CIDs are unchanged by construction, since they were not stale sources. The files list must stay complete (including the main spec and every appendix) so the appendix and main-file promote checks keep passing. The rewrite is assembled with `specflowctl cache-write` (`--basis delta`, or `--basis repair` for a failure-record recovery): the agent re-declares every entry's path and dependency scope, the tooling recomputes the CIDs (unchanged by construction for carried-over judgments) and writes the file (see `framework/validation_cache.md` §Write Rules → Tooled writes).
- The report uses the standard skeleton with `mode: delta`, an `Incremental scope:` section (re-run checks + carried-over declaration + cross-check — rules have no cross-check), and the cache-write note "cache written with basis: delta".

### Failure recovery

A delta run over a **failure record** (`result: fail` + `blocking: true`; validate/verify/review delta FAIL writes it, and verify/review candidate full FAIL writes it too — a full-run record with `basis: full`; review's blocking cache is the same shape) restores the pass cache incrementally after the findings are resolved. This is the counterpart of the stale-cache recovery — same command, same trust model, different baseline. The split between which full FAILs write a record and which delete the cache is defined by gate role (see §Failure handling by gate role in `framework/validation_cache.md`); the recovery itself is identical regardless of `basis`.

1. **Scope derivation** — first, read the record's per-check `checks` entries and determine whether it carries a `status` map:
   - **No status map on a failure baseline** — the record cannot say which judgments failed, so **nothing may be carried over**: the re-run set is every declared judgment (plus the cross-check for unit targets). Report the degradation ("the failure record carries no per-check status map — the recovery is a full re-run") and ask whether to proceed or run the full command instead. This is the safe path for legacy blocking caches (written before the failure-recovery design) and for any fail/blocking record written without a status map — absent status means **pass** only on a pass baseline, never on a failure baseline.
   - **Status map present** — the re-run set = {judgments whose `status` is `fail` in the failure record} ∪ {judgments whose declared deps went stale against the current content (the fix changed content)} ∪ {judgments a targeted run has since found P0/P1 in — the targeted finding overrides the recorded status: the record's `pass`/`carried` evidence for that judgment was contradicted by a complete single-check judgment, so the recovery must re-run it} ∪ {the cross-check}. Judgments with `status` `pass`/`carried` are carried over only when their evidence is unchanged by construction — a targeted P0/P1 on a judgment is a change of that kind even when the content did not move, so the judgment joins the re-run set instead of being carried. A targeted finding that cannot be mapped to a recorded judgment (the keyword matched no recorded check key) degrades the recovery to a full re-run — nothing is carried over, matching the no-status-map path. A full-run record's status map has only `pass`/`fail` values (every judgment was re-executed by the full run; `carried` appears only in delta-FAIL records). The same derivation rules as the stale-scope path apply to the newly stale sources (unclaimed entries map by the command's fixed association, §Incremental scope); when the set covers every declared check, report the degradation ("the failed judgments plus the fix's impact cover every check — the recovery is a full re-run").
2. **Execution** — re-run the derived set. Any P0/P1 finding updates the failure record (fresh evidence, findings body, status map — the record stays). A full pass rewrites the cache with `mode: full`, `basis: repair`: new evidence for the re-run judgments, the original evidence for the carried-over ones, fresh `timestamp`. The report declares the recovery scope explicitly before executing, as for any delta run.
3. **No stale source required** — the failed judgments are the re-run reason; the "cache is fresh" stop does not apply to a failure baseline.
4. **Precondition** — the failure record must be present and its dependency evidence must still hold for the carried-over judgments (a stale record's carried-over evidence is untrusted: derive the scope from the stale sources per §Incremental scope and re-check conservatively — declare-heavy). A MISSING record is not a baseline: run the full command. A record without a per-check `status` map is not a failure-recovery baseline for any judgment — step 1's no-status-map path degrades it to a full re-run (fail-closed: carrying over unresolved P0/P1 findings is not allowed).

The failure record's `basis` records its writer: `delta` for a delta-FAIL record, `full` for a full-run record (verify/review candidate full FAIL, stable-only confirmation FAIL). The recovery's pass cache is marked `basis: repair` for audit separation. Promote trusts a `basis: repair` cache exactly like `full`/`delta` — the evidence rules are identical (see §Trust statement).

### Trust statement

A delta or repair cache carries no lower evidence standard than a full cache: every judgment in it is backed by dependency evidence present in the cache at promote time — re-run judgments by their new CIDs, carried-over judgments by their unchanged CIDs. The gate verifies the same mechanical checks for all three. `basis: delta` / `basis: repair` is audit metadata (see `framework/validation_cache.md` §Format); the promote gate does not distinguish full, delta, or repair.

### Layer applicability

Delta re-runs apply to **candidate targets** and to **stable-only targets with a usable baseline**. For a candidate target, a delta run restores promote eligibility, which exists only for the candidate layer. For a stable-only target, a delta run restores the stale confirmation state — it is a recovery, not a complete re-confirmation: it re-runs only the judgments whose dependency evidence went stale and carries the rest over from the pass baseline. The precondition is the same as for candidates — the gate's confirmation cache must exist with `mode: full` and `result: pass` (review: `blocking: false`). A MISSING cache (never confirmed) has no usable baseline — report: "No usable confirmation baseline. Run the full `validate@{unit}` / `verify@{unit}` / `review@{unit}` (confirmation check) first, or `specflowctl fork` to start a new round." A blocked/failed confirmation cache is a failure record: the failure recovery applies to stable-only targets the same way (restoring the confirmation state with `basis: repair`); when the stable content itself can no longer hold against the changed dependency or rule, the record stays and forking reconciles it (see §Stable-only Targets → On FAIL).

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

**Targeted runs never write a cache.** They are iterative feedback only. The promote gate accepts only caches from full runs. If a targeted run finds P0/P1 findings, it deletes a pass cache — blocking findings at any granularity mean promote must not proceed; an existing failure record is kept (it is already blocking and is the failure-recovery baseline), but the targeted finding **invalidates the record's `pass`/`carried` status for that judgment** — the failure-recovery delta run must re-run it instead of carrying it over (see §Delta Runs → Failure recovery).

**Targeted execution shape:** targeted runs execute directly in the main agent session — no sub-agent is launched and no prompt is assembled per §Sub-agent Prompt Assembly. They are lightweight single-check feedback outside the promote gate, so the independent-session guarantee does not apply to them.

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
| `validate@{target}` full FAIL (candidate) | Delete the validate cache (full-run failure — trust establishment failed; no failure record). The spec is the upstream root — nothing may be carried over from an unconfirmed spec (see §Failure handling by gate role in `framework/validation_cache.md`) |
| `validate@{target}` full FAIL (stable-only) | Write a failure record (`result: fail` + `blocking: true`) and recommend forking (see §Stable-only Targets) |
| `verify@{unit}` full PASS (all aligned) | Write `verify_result.md` with `result: pass`, `mode: full`, `blocking: false`, `hash` + `deps` dependency evidence |
| `verify@{unit}` full PASS (P2/P3 non-blocking findings) | Write `verify_result.md` with `result: pass`, `mode: full`, `blocking: false`, severity counts (`p0_count`...`p3_count`), `hash` + `deps` dependency evidence. Promote may proceed |
| `verify` full FAIL (any P0/P1 findings, candidate) | Write a failure record (`result: fail` + `blocking: true`, per-item `status` map) — the `reverify@{unit}` failure-recovery baseline. Agent must stop, not proceed to promote. (Anchored to the validated spec, so `pass` items with unchanged evidence may be carried over — see §Failure handling by gate role) |
| `verify` full FAIL (stable-only) | Write a failure record; report the drift and recommend forking |
| `review@{unit}` full PASS | Write `review_result.md` with `mode: full`, `blocking: false`, `hash` + `deps` dependency evidence for all read files, findings body |
| `review@{unit}` full FAIL (P0/P1 found) | Write `review_result.md` with `mode: full`, `blocking: true`, finding counts, findings body, and the per-dimension `status` map (`pass`/`fail` for every review dimension — full runs have no `carried`; the map is the failure-recovery scope input) |
| `revalidate@{target}` / `reverify@{unit}` / `rereview@{unit}` delta PASS | Rewrite the gate's cache with `mode: full`, `basis: delta`: new evidence for re-run judgments, original evidence for carried-over judgments (see §Delta Runs) |
| Delta run FAIL (P0/P1 findings) | Write a failure record — rewrite the gate's cache with `result: fail`, `blocking: true`, severity counts, findings body, `basis: delta`, and the per-check `status` map. Promote must not proceed (see §Delta Runs → Failure recovery) |
| Any targeted run (`:check-{n}` / `:{keyword}`) | PASS with only P2/P3 findings → report findings, write NOTHING. P0/P1 findings → delete a pass cache; keep an existing failure record (it is already blocking and is the recovery baseline). Targeted runs never write a cache |

### Cache semantics

- A cache exists only when a complete-coverage run (full or delta) completed. `mode: full` is the only value ever written; `basis` records whether the cache came from a full run (`full` or absent), an incremental one from a pass baseline (`delta`), or an incremental recovery from a failure record (`repair`).
- A targeted run never writes or downgrades an existing cache — a full/delta cache stays valid unless a targeted run finds P0/P1 (which deletes a pass cache; a failure record is kept).
- Any FAIL at full granularity deletes the candidate cache for validate (full-run failure — trust establishment failed; the spec is the upstream root, see §Failure handling by gate role in `framework/validation_cache.md`); verify and review full FAILs write a failure record. Any FAIL at delta granularity writes a failure record for all three gates. P0/P1 at any granularity means promote must not proceed.

## Relationship with Promote

- `specflowctl promote --unit <name>` requires fresh caches for validate, verify, review, and appendix coverage. Every cache has `mode: full` by construction (targeted runs do not write caches; delta runs keep `mode: full` and record `basis: delta`/`repair` — the promote gate checks the same evidence for all three).
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

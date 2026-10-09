# Validate Checklist

## Overview

When an agent executes `validate@{unit}`, it uses the 10 checks defined here. The trigger route in `framework/concepts.md` loads this file at validate time, not proactively.

## Prerequisite — Read all unit files

Before executing any check, the agent must read the complete unit content:

1. Read the main spec: `docs/specs/units/candidate/unit_{unit}.md`
2. Glob all candidate appendix files: `docs/specs/units/candidate/appendix/unit_{unit}_*.md`
3. For each appendix, read its frontmatter. Skip exempt content; confirm appendix ownership by its `unit` field.
4. Read the content of every non-exempt appendix file.

The unit's complete spec is the union of the main spec and all non-exempt appendix files. All checks that follow operate on this union.

## Mode Selection

**Mechanical pre-pass:** for a candidate target, every full and delta `validate@{unit}` run starts with `specflowctl validate candidate --unit {name}` (see Check 1). A mechanical FAIL stops the run — fix and re-run the tool before planning; no gate run is planned and no session launches until it passes.

| Trigger | Mode | What to execute |
|---------|------|-----------------|
| `validate@{unit}` | full | All 10 checks, grouped into 5 coverage keys by read surface: `structural`, `design`, `acceptance`, `dependencies`, `clarity`. Quality checks are holistic — always runs full. When relationships are assigned or the primary pass produced findings, the final session runs before finalize. |
| `validate@{unit}:check-{n}` | targeted | Single check `{n}` only. User explicitly chooses focus. Does not write a cache. |
| `validate@{unit}:{keyword}` | targeted | Match keyword to check name (e.g., "design" → Check 2, "scope" → Check 3). User explicitly chooses focus. Does not write a cache. |

**Keyword domain:** validate keywords resolve to check names — `structure` (Check 1), `design` (Check 2), `scope` (Check 3), `evidence` (Check 4), `acceptance`/`coverage` (Check 5), `affects` (Check 6), `cross-unit` (Check 7), `constraint` (Check 8), `ownership`/`surface` (Check 9), `clarity`/`clear` (Check 10). A keyword matching no check name is a no-match — ask the user for clarification.

**Output:** Targeted runs report only the executed check(s) and note "This was a targeted check — no complete cache was written. Run `validate@{unit}` for a complete validation." A targeted P0/P1 must first be persisted with `gate-invalidate --check {check key}`.

### Stable-only mode

When no candidate spec exists (validate against stable), run the same 10 checks against the **stable** content:

1. Read the stable main spec: `docs/specs/units/stable/unit_{unit}.md`
2. Glob all stable appendix files: `docs/specs/units/stable/appendix/unit_{unit}_*.md`, read every non-exempt appendix (same skip rules as the candidate path)
3. Run all 10 checks against the stable content — Checks 6/7/8/9 are the live part: referenced files, dependency-unit contracts, rules, and declared surfaces may have changed since promote, so the stable content may no longer hold (e.g. a new rule now prohibits something the stable design does, or another unit now declares a file this surface also declares)
4. **PASS** → `gate-finalize` writes the validate cache with `target: stable` (confirmation state consumed by `fresh@stable`; `mode: full`, whole-file hash + chunk evidence; same coverage sequence as Step 9)
5. **FAIL** → `gate-finalize` writes a failure record (`result: fail` + `blocking: true`, `mode: full`, `basis: full`, and the per-check `status` map — `pass`/`fail` for every executed check; full runs re-execute local checks and may carry unchanged relationship judgments — the confirmation state stays visible as BLOCKED and is the failure-recovery baseline), present the findings (Check 2 Step 1 and 5a/5h FAIL findings re-verified per §Step 9 → Extraction re-verification before presentation), and recommend forking the unit (`specflowctl fork --unit <name>`) to reconcile the stable content with the changed dependency or rule. Do not edit stable directly; normal candidate-to-stable writes use promote (the routed remove/update procedures own their narrow exceptions)

The stable confirmation cache is read-only state: it grants no promote eligibility (stable has no gate). Delta re-runs (`revalidate`) apply to stable-only targets with a usable baseline — a pass cache (`result: pass`, STALE recovery) or a failure record (failure-record recovery); a MISSING stable cache needs the full confirmation run — see `framework/verification_scope.md` §Stable-only Targets and §Delta Runs → Layer applicability.

## Execution Rules

- **Subagent permissions:** validate executes one lens-pure session batch per independent read-only sub-agent session — the sub-agent may inspect file content, search text by pattern, and locate files by name pattern. Must NOT modify files, execute commands, or delegate to other agents. The main agent sends `specflowctl gate-mission --run {run_id} --keys {keys} --format prompt` output verbatim; its check keys are the session scope (see `framework/verification_scope.md` §Coverage Model). Targeted runs (`:check-{n}` / `:{keyword}`) execute directly in the main agent session instead; they never launch a sub-agent and never create a coverage run or cache write.
- Each check reports **PASS**, **WARNING**, or **FAIL** with a reason.
- On FAIL, the agent must identify **which information sources contradict each other** (e.g., "spec body describes auto-retry logic but no acceptance item covers it") in the FAIL reason, and record the fix in the FAIL reason. When the finding is written out, its `evidence:` block quotes the contradicting sources verbatim and its `fix:` (or `decision:`) field states the repair — the entry line's location reference is a pointer, not the evidence.
- Resolution types:
  - **actionable** — A concrete repair can be made inside the current candidate spec without user judgment.
  - **needs_decision** — Requires user input (unclear intent, missing decision, or external dependency). Stop and ask.

## Output Format

==ATOM_BEGIN:report_skeleton==
## Unified Report Skeleton

All quality-gate reports (validate and verify) share the same report skeleton below. The header lines (`Result`, `Blocking promote`, `Key counts`), the Findings block fields, and the `Next step` line are identical across gates; only the body content and the gate-specific extra lines inside each finding block are gate-specific and defined in each checklist file. The Findings section follows the header because on FAIL it is the decision surface — the body is its audit and evidence.

```
────────────────────────────────────────────
{gate}@{target} · {mode} · {layer}
Result: PASS | FAIL
Blocking promote: yes | no
Key counts: Findings: N (P0: a | P1: b | P2: c | P3: d)
────────────────────────────────────────────
Findings: none
# or, one entry per finding:
Findings:
  [{severity}] {location} — {issue} (actionable | needs_decision)
    problem: {one-sentence statement of what is wrong, naming the concrete subject}
    evidence:
      - {verbatim quoted source content} — {source label}
    impact: {what goes wrong or stays undecidable if this is not resolved}
    fix: {concrete repair action}                  # actionable
    # or
    decision: {the question the user must answer}  # needs_decision
    options:
      - {candidate option}
    {gate-specific detail lines}
    ref: {anchor or line reference}                # optional, tracking only
────────────────────────────────────────────
{body}
────────────────────────────────────────────
Next step: {concrete next command with reason, or "None"}
────────────────────────────────────────────
```

**Field definitions:**

- `{gate}@{target}` — the gate and target that produced this report, e.g. `validate@user_auth`, `verify@user_auth`. Gates: `validate`, `verify`. Targets: unit or rule name. The `verify` gate carries two lenses — `alignment` (spec-vs-code) and `quality` (spec-aware code quality) — and its sessions report under one of them.
- `{mode}` — `full` for full runs; `targeted (user requested: {keyword})` for targeted runs; `delta` for reviewed re-runs (`revalidate@{target}` / `reverify@{unit}`). The delta review session has its own contract (`Review result:` + optional `Recheck:` line) and is not rendered with this skeleton.
- `{layer}` — the spec layer checked: `candidate` | `stable`.
- `Result` — `PASS | FAIL` for both gates. For coverage runs this value is generated by `gate-finalize` from the accepted sessions (or from the single rule-validate session); the coordinator never supplies it. The gate is decided by severity: FAIL means gate-driving retained P0/P1 findings exist. validate grades findings P0/P1 only, so any validate FAIL is a P0/P1 finding; verify FAIL means gate-driving retained P0/P1 mismatches/findings exist. A finding deferred to another unit is retained and routed but is not gate-driving — it never causes FAIL.
- `Blocking promote: yes | no` — `yes` when the run FAILs (gate-driving P0/P1 findings exist); `no` otherwise. Valid for both gates.
- `Key counts: Findings: N (P0: a | P1: b | P2: c | P3: d)` — N is the gate-driving retained finding count and a/b/c/d are generated by `gate-finalize`; the coordinator never supplies them. validate grades findings P0/P1 only, so its P2/P3 counts are always 0; verify's blocking mismatches equal the P0/P1 counts and non-blocking mismatches equal the P2/P3 counts. A finding deferred to another unit is excluded from these counts (it is presented under `Deferred to {unit}:`). Gate-specific summary numbers (validate's failed checks and advisory findings, verify's coverage, the quality lens's suppressed-by-spec count) appear in the body.
- `{body}` — gate-specific content defined in this file's body format section (validate: one line per check; verify `alignment`: per-item alignment and divergence analysis; verify `quality`: architecture assessment and suppressed findings).
- `Findings:` — `Findings: none` when no finding is retained; otherwise one entry per finding: a unified entry line `[{severity}] {location} — {issue} (actionable | needs_decision)` followed by the finding block's required detail lines. `actionable` — a concrete repair can be made without user judgment (alignment: direction spec_gap/code_gap; quality: a determined recommendation); `needs_decision` — requires user input or a design decision before the fix can be made (alignment: direction needs_design/blocked; quality: architecture trade-offs). The block is written for a reader who has not opened the spec or the code: delete every `ref:` line and every `§`, line-number, or other anchor reference from the field text, and the reader must still learn what is wrong, what the conflicting sides say, what is at risk, and what to fix or decide. Anchors appear only in the optional trailing `ref:` line, which carries tracking information and no meaning.
  - `problem:` — one sentence naming the concrete subject (acceptance item id, field or parameter name, behavior) and what is wrong with it; never an anchor, region name, or pointer.
  - `evidence:` — the quoted content that proves the claim, as one or more indented `- {verbatim quoted source content} — {source label}` sub-lines. Contradiction findings quote both conflicting sides verbatim; undefined or missing behavior lists the defined part and the missing part; spec-vs-code findings quote the declared behavior and the implemented behavior. A quality P3 finding uses its `fact_anchor:` line as this evidence form.
  - `impact:` — what goes wrong, or stays undecidable, if the finding is not resolved.
  - `fix:` — actionable findings: the concrete repair action. `decision:` — needs_decision findings: the question the user must answer, with `options:` sub-lines listing the candidate choices.
  - The block's detail lines are contiguous indented lines directly under the entry line — no blank lines inside the block (the block is stored and re-rendered verbatim when a finding is carried into a delta/repair run).
  - `{gate-specific detail lines}` — gate-specific extras follow the shared fields: alignment `root_cause:`, `direction:`, and `confidence:`; quality `spec_context:` and its mechanically required `fact_anchor:` for P3 findings. validate and rule validate add none.
  - Findings are grouped into the batch group and decision group defined in this file's batch classification section when this file defines one; flat when this file defines no batch classification or grouping is inactive. Batch-group entries carry the same fields; terse one-line values are fine.
  - Findings whose recorded ownership routes them to another unit are presented in a `Deferred to {unit}:` section after the Findings section, with the same block fields plus an `ownership:` line. They are retained and routed but do not count toward `Key counts`, `Blocking promote`, or the gate result (see `framework/unit_verify_checklist.md` §Output Format → Deferred findings).
  - Each finding's `[{severity}]` is assigned by the session that raises the finding. When the final synthesis (`framework/verification_scope.md` §Final synthesis) retains or merges findings, it may raise a retained finding's canonical severity conservatively — never lower it; counts and blocking derive from the canonical severities (see `framework/severity_policy.md`).
- `Evidence recording:` — reports carry no dependency declarations. `gate-finalize` records the run's whole input surface: one cache entry per input-surface file with its whole-file `hash` and ordered chunk sequence, plus a per-check marker (`check` + lens, and the failure status on failure records) for every executed judgment. Spec objects resolved by name are recorded as logical references so a layer move does not stale the cache (see `framework/validation_cache.md` §Format → Entries). Delta/repair runs record the re-run keys' markers alongside the carried keys' snapshotted markers; targeted runs write no cache.
- `Change review:` — delta/repair runs only (mode `delta`). The recorded review outcome: the change-set summary (changed files and, per file, the localized change kinds and line spans), the `Review result:` (accept | recheck | escalate-full), and when the reviewer named re-runs, the re-run keys in the run's own structure (e.g. validate: "check-2 (design group): re-run — Description section changed") followed by a line declaring the carried conclusions ("all other conclusions: carried over — recorded content unchanged"). The change set is derived mechanically at plan time (ordered bidirectional chunk diff); the reviewer accepts, names re-run keys, or escalates (see `framework/verification_scope.md` §Delta Runs). The mechanical floor re-runs invalidated verify records, failed/invalidated judgments (repair), current keys the baseline never declared, and explicit `--rerun` overrides regardless of the review. A failure record whose per-check status map is absent, invalid, or inconsistent with its judgment baseline, or an invalidated key that cannot map to the current surface, is covered by a full-scope plan — nothing is carried over. The review result and the planned scope are reported by `gate-plan` before execution begins (the user must see what will be re-run and what will be carried over) and again in the final report.
- `Next step:` — the concrete command to run next with its reason; `None` when nothing further is needed. A finding's fix lifecycle has three states with fixed wording: `finding_open` → "Resolve the findings, then re-run the target-appropriate re-check command (`validate@{target}:check-{n}`; unit targets also `verify@{target}:{keyword}`) to confirm"; `fixed_pending_recheck` → "Fixes applied; re-run the target-appropriate re-check command to confirm." — only after the approved fix was actually written; `verified` → "Re-check passed." — only after a re-check confirmed the fix. A gate report is always produced before any fix is applied (nothing is implemented before the user approves the findings), so an actionable finding's report-time `Next step` is always the `finding_open` wording. Other guidance: both gates green → "if the design is finalized, run `promote@{target}`"; needs_decision → "awaiting your decision on {item}"; nothing further → `None`.

**Targeted runs:** end the report with the gate's targeted note ("This was a targeted check — no complete cache was written. Run `{gate}@{target}` for a complete ...") after the `Next step` line. If the result contains P0/P1, run `gate-invalidate` before reporting completion.

### Completion — Persist Gate Cache

A quality-gate run is complete only when its result is persisted and visible to `fresh`/`promote`. A report alone does not satisfy the gate.

- **Full (`validate@{target}` / `verify@{unit}`, `mode: full`, `basis: full`)** — complete only when the coverage run finished and its cache was written: (1) `specflowctl gate-plan --gate {validate|verify} (--unit {name} | --rule {id}) --target {candidate|stable} [--relationships NAMES|none] [--inputs-file PATH]` fixed the immutable input snapshot, local coverage set, and relationship scope (`--relationships` is required for unit runs; select names or `none`) **before any executor read input** (before mission assembly / sub-agent launch) — the input manifest adds evidence that every session may read but never creates a coverage key, and it must not be a repository-content file (keep it under the ignored `meta/plan_inputs/` or outside the repository); (2) the agent partitioned uncovered coverage into batches with the same kind and lens, reusing accepted public tasks and waiting for assigned tasks and, for each batch, generated a mission with `specflowctl gate-mission --run <run_id> --keys <k1,k2,...> --format prompt`, sent it verbatim to one independent reviewer, and recorded its report with `specflowctl gate-submit --run <run_id> --session <session_id> --keys <k1,k2,...> --report PATH` until `gate-status` reports full coverage; (3) when relationships are assigned or the primary pass produced findings, one independent final synthesis was generated with `specflowctl gate-mission --run <run_id> --final --format prompt` and recorded with `specflowctl gate-submit --run <run_id> --session cross --keys cross --report PATH`; a run with neither assigned relationships nor findings skips this step; (4) the coordinator ran `specflowctl gate-finalize --run <run_id>` and the cache `docs/specs/meta/validation/{unit|rule}/{name}/validate_result.md` | `verify_result.md` was written (the merged verify cache carries its `alignment` and `quality` sections); (5) `fresh@{target}` (`fresh --unit {name}` / `fresh --rule {id}`) shows the expected gate state (`FRESH` for `result: pass` / `blocking: false`, `BLOCKED` for `result: fail` / `blocking: true`). A candidate **full-run** FAIL writes a failure record (`result: fail` / `blocking: true`; validate/verify records declare `pass`/`fail` for every executed judgment) — the failure-recovery baseline. A finalize that finds any input changed since `gate-plan` is rejected and writes no cache. See `framework/validation_cache.md` §Write Rules and §Failure handling by gate role.
- **Delta (`revalidate@{target}` / `reverify@{unit}`, `mode: full`, `basis: delta` or `basis: repair`)** — the same coverage-run sequence plus one review session: `specflowctl gate-plan --mode delta` (or `--mode repair` from a failure record) computes the change set, schedules the review, and mechanically forces invalidated/failed/new keys; the review report (`Review result:` + optional `Recheck:`) may add re-run keys; `gate-finalize` merges the reviewed scope's fresh evidence with the carried baseline evidence and records the review. See `framework/verification_scope.md` §Delta Runs.
- **Targeted (`:check-{n}` / `:{keyword}`, no complete cache)** — complete when the report is produced and, for P0/P1, the coordinator has run `specflowctl gate-invalidate` for every affected check key. Targeted runs intentionally do NOT create a gate run — `gate-plan` / `gate-mission` / `gate-submit` / `gate-finalize` are not run. A targeted PASS leaves cache state unchanged. A targeted P0/P1 deletes a pass cache or persists `invalidated_checks` on a failure record and invalidates any matching open run. For verify, the same locked transition invalidates the contradicted immutable evidence, so consuming caches and open runs must recheck the affected judgments. It never publishes a complete result cache or satisfies the promote gate. The report ends with `This was a targeted check — no complete cache was written. Run {gate}@{target} for a complete ...`.

**Self-check:** `ls docs/specs/meta/validation/{unit|rule}/{name}/` and `fresh@{target}` (unit: `fresh --unit {name}`, rule: `fresh --rule {id}`).
==ATOM_END:report_skeleton==

### Body format (validate)

One line per check, numbered as in this file:

```
1. Structural integrity: PASS | WARNING | FAIL — reason
2. Design soundness: PASS | FAIL — reason
3. Scope integrity: PASS | WARNING | FAIL — reason
4. Evidence-driven vs design-driven consistency: PASS | FAIL — reason
5. Acceptance coverage & correctness: PASS | FAIL — reason
  5a. Coverage & item-set correspondence: PASS | FAIL — reason
  5b. Semantic consistency: PASS | FAIL — reason
  5e. Item substance: PASS | FAIL — reason
  5h. Contract statement carry-over: PASS | FAIL — reason
6. Affects-source validity: PASS | FAIL — reason
7. Cross-unit consistency: PASS | WARNING | FAIL — reason
8. Constraint alignment: PASS | FAIL — reason
9. File associations (mechanical): PASS | FAIL — reason
10. Clarity: PASS | WARNING | FAIL — reason
```

When a final session ran, report its summary: `Cross-check: N/M PASS|FAIL — reason` for M assigned relationships, or `Cross-check: PASS|FAIL — reason` for finding disposition only. Do not print results for unassigned relationships. Then report the counts:

```
Failed checks: N | Advisory findings: K
```

**Counting rules:**
- `Findings: N (P0: a | P1: b | P2: c | P3: d)` — N is the total number of distinct findings across all FAIL checks (quality-bar findings merged per the per-item merge rule, see Per-item merge rule below); a/b/c/d the count per severity. validate grades findings P0/P1 only — P1 is the contract-decided default; a P0 grade requires the §9 boundary check (see Severity handling below) — so `c` and `d` are always 0. In targeted runs, only executed checks are counted.
- `Failed checks` is the number of FAIL checks among executed checks, shown in the body's check lines. WARNING is not a failed check.
- `Advisory findings` (Check 1 step 3 hygiene WARNING) are presented on the check line's reason and counted separately as `Advisory findings: K` in the body. They are never counted in `Findings` and never affect `Failed checks`.
- The same counts are reused in the Present Findings summary (`Findings` N = batch group items + decision group items).

**Multi-finding enumeration:** When a FAIL reason contains multiple distinct findings, list each finding under the check line as its own entry in the unified finding format `[{severity}] {location} — {finding} (actionable | needs_decision)`, followed by the shared finding block (§Output Format). The entry line must begin with the bracketed severity after optional indentation — the parser accepts indentation or a single leading `-`, but not a numbered prefix, so the session report keeps entries as standalone lines; the presented summary may re-number them (`5a-1`, `5a-2`, ...) as presentation only. Each entry carries a location reference (the contradicting information sources, per Execution Rules), the finding statement, its resolution type, and the block fields:

```
5a. Coverage & item-set correspondence: FAIL — 3 findings
  [P1] {location} — {finding} (actionable)
    problem: ...
    evidence:
      - ...
    impact: ...
    fix: ...
  [P1] {location} — {finding} (actionable)
    ...
  [P1] {location} — {finding} (needs_decision)
    ...
```

A check with a single finding keeps the existing one-line reason format plus the finding entry and block.

**Per-item merge rule:** Findings from sub-check 5e that reference the same acceptance item (same location) and whose fix is a rewrite of that item are merged into **one finding**: the merged entry's issue line summarizes the item defect, and its `evidence:` block quotes each violated rule and the offending text. The merged entry's severity is the highest among its violated rules (P0 > P1); a P0 grade still requires the §9 boundary check per the Severity handling section. Findings from 5a (spec body regions), 5b (each contradiction is an independent fix edit), and 5h (prose statements) are not merged — their locations and fixes differ.

When findings mix resolution types (within one check or across checks), the report presents each finding with its own resolution — a `needs_decision` finding stops the flow and requires user input per Execution Rules.

**Evidence discipline:** every check line's reason must reference the spec content the verdict covers (section title, item id, appendix name). A PASS with no reference means the check did not run — report FAIL with "no evidence basis". The check-line reference is the verdict trace; a finding separately carries the quoted content in its `evidence:` block.

---

## Check 1 — Structural integrity

**Purpose:** Verify the file is parseable and all required fields exist, as a prerequisite for all subsequent checks.

**Mechanical pre-pass (candidate targets, before gate-plan):** run `specflowctl validate candidate --unit {name}` first. The tool deterministically enforces: frontmatter fields; acceptance item schema (required fields; `testable` items need a Gherkin-style Given/When/Then description, and `.feature` syntax is rejected) and `implementation_surface` resolution; `affects.files` and `affects.evidence_files` path existence; `unit_refs`/`rule_refs`/appendix existence; candidate-layer spec paths in the body and every non-exempt appendix; section/region locatability (at least one `##` heading, unique headings, malformed headings, frontmatter region purity, unique non-empty item ids); dependency cycles; prose path hygiene (WARNING); and environment-specific content — developer-machine absolute paths, local addresses, and credential patterns (narrative hits FAIL; unmarked fenced hits WARNING). A mechanical FAIL stops the run before any session launches — fix and re-run the tool. Warnings are passed to the structural session for confirmation.

**Agent-judged residue (the structural session executes these):**

1. Read `docs/specs/units/candidate/unit_{unit}.md` and all non-exempt appendix files (see Prerequisite)
2. **Stable-layer spec paths in prose (FAIL):** the tool flags candidate-layer paths everywhere but cannot distinguish structured fields from prose for stable-layer paths — a stable-layer spec path (`docs/specs/units/stable/...`, `docs/specs/rules/stable/...`, or a relative `stable/...*.md` form) in narrative prose mispoints after the referenced unit promotes. Structured fields (`implementation_surface`, `affects.files`, `affects.appendices`, `affects.dependencies`) may hold stable-layer paths. If found → FAIL with quoted path, section, and line reference. Reference appendix files and other specs by concept name or file name instead
3. **Prose-path hygiene confirmation (WARNING):** review the tool's WARNING hits plus any source-path pattern the scanner cannot express — relocate to `implementation_surface`, `affects.files`, or `affects.evidence_files`, or convert to a concept name reference. Exclusions mirror the tool's: structured fields, `framework/` governance paths, `docs/specs/meta/` cache paths, marked fenced examples
4. **Region structure semantics check (FAIL):** the section regions must match the content's semantic structure — the acceptance item set's boundary and the item-region pins of item judgments are only as correct as the split. Reading the spec itself, verify:
    - **The acceptance_item_set region covers every real item** — the region runs from the exact `acceptance_item_set:` marker line to the next `##` heading outside a code fence (the enclosing section's end), or through the last real line of the file. `###` and deeper headings never terminate the set: an item separated from the previous one by a `###` subheading is still part of the set and is verified. An item placed after the enclosing section's `##` heading sits outside the set and cannot be validated as an item — catch that here. Distinguish real items from fenced example blocks — a fenced `- id:` example is content, not an item (the mechanical pre-pass enforces the exact-marker and heading-format preconditions; the semantic coverage judgment is this step's)
    - **Fenced code blocks are content** — `##`-like lines inside ``` / ~~~ fences must not split regions; when a fence would visually span a section boundary, the spec needs restructuring (a fence cannot cross `##` headings — close the fence before the next heading)
    - **Headings are unique and well-formed** — every `##` heading resolves uniquely (a duplicated heading cannot locate an item set or an item region; the mechanical pre-pass enforces the formal preconditions, the semantic judgment is this step's)
    - Fix direction: restructure the spec so the split is semantically clean (cohesion per `framework/spec_writing_guide.md` §13)
5. **Environment agnosticism judgment (FAIL):** the mechanical scan flags the unambiguous patterns; the session judges the residue — whether a narrative value is a hard requirement (FAIL) or an explicitly marked example placeholder (pass), and credential-like content the pattern scan cannot recognize. Must NOT contain fixed local machine IP addresses or ports as hard requirements, or live or environment-specific credentials, tokens, or private secrets. If found → FAIL with quoted text, section name, and line reference (actionable: replace with project-scoped relative paths, abstract placeholders, or configuration-driven parameters)

**PASS:** The mechanical pre-pass passed and the residue judgments found no defect

**WARNING:** Prose-path hygiene hits and unmarked fenced environment content — surfaced by the tool, confirmed or cleared by the session

**FAIL:** Any mechanical pre-pass failure, a stable-layer spec path in narrative prose, or environment-specific hardcoding in prose or structured fields (actionable)

**Check method:** Deterministic tool pre-pass (`specflowctl validate candidate`) + agent judgment on the semantic residue

**Communication note:** When suggesting Check 1 to a user, describe it as "structural integrity — verifies file structure and reference existence without evaluating design quality."

Deletion follows `framework/removal_workflow.md`; existing units use the normal checks.

---

## Check 2 — Design soundness

**Purpose:** Evaluate whether the design itself is correct and reasonable — not just whether it is well-documented — and whether the spec satisfies the decision-closure half of the `framework/spec_writing_guide.md` §9 Authoring Baseline: every implementation-affecting decision is closed, so the downstream executor is never forced to choose, and any intentionally unmade decision declares its boundary. The clarity of the spec's expression is judged separately by the clarity check (Check 10), which reads the spec and reports what is unclear, underspecified, or internally contradictory. The subagent must actively reason about the design, not passively verify documentation completeness. Appendix content describing design decisions, API contracts, component trees, or data types is part of the unit's design and must be included in this analysis.

**Execution steps:**

**Step 1 — Goal-means analysis**
- Read the unit's goal and scope from the main spec AND appendix files
- For each major behavior described (in main spec or appendices): does it demonstrably serve a stated goal? If a behavior cannot be traced to any goal → flag (possible over-engineering)
- Reversely: is the goal achievable by implementing all described behaviors? If implementing everything still does not meet the goal → flag (design gap)
- Check whether any behavior violates a stated non-goal (e.g., non-goal says "no multi-tenancy this round" but the behavior describes tenant isolation)
- **Proportionality check:** Is the design complexity proportional to the stated goal? If the same goal could be achieved with significantly less design surface area → flag (possible over-engineering)

Each Step 1 flag is a FAIL finding at **default severity P1** and must carry an extraction artifact that makes the claim falsifiable:
- **Source quotes:** the goal or non-goal declaration, and the flagged behavior or design surface, each quoted from the spec union.
- **Connection judgment:** why the behavior serves no stated goal / why the described behaviors cannot meet the goal / why the behavior violates the non-goal. For the proportionality flag: the smaller design surface that would achieve the same goal, and why the goal still holds under it.
- A finding without this artifact is not presented — the independent final synthesis re-verifies the artifact before classification and marks unfaithful claims suppressed (see §Step 9 → Extraction re-verification; e.g. a behavior that does serve a stated goal, or a proportionality claim that rests on preference rather than an establishable smaller surface, drops the finding).

**Step 2 — Design rationale review**
- **Evidence-driven precondition (per acceptance item):** The waiver is decided per acceptance item, not per spec. Read the spec frontmatter's `evidence_appendix_ref` field and each acceptance item's `affects.appendices`.
  - For each acceptance item: if `evidence_appendix_ref` is PRESENT and not `none` AND the item's `affects.appendices` references the evidence appendix → the item is evidence-driven (its behavior domain is recorded from existing implementation). The code behavior itself constitutes the design rationale. **Skip** the rationale review below for this item. Report per item: "Step 2: waived (evidence-driven — rationale is implicit in existing code)".
  - Otherwise → the item is design-driven. Execute the rationale review below for this item.
  - Mixed states are legal: a spec may combine evidence-driven and design-driven items during incremental replacement (see `framework/operations/adopt.md`). Report the classification per item.
- Does the spec explain **why** each key design decision was made? (e.g., "chose event-driven architecture because async decoupling is required, not because it is popular")
- If there are viable alternative approaches (sync vs async, push vs pull, strong vs eventual consistency), does the spec acknowledge them and explain why they were rejected?
- If a design choice is non-obvious and no rationale is given → FAIL (actionable: add design decision record)

**Step 3 — Adversarial analysis (red-team)**
Actively search for design flaws in both main spec and appendix content. Read appendix descriptions of API contracts, data formats, state machines, error handling, and include them in each attack angle:

| Attack angle | Questions to ask |
|---|---|
| Dependency failure | What happens when a dependency returns an error, times out, or crashes? Is fallback or degradation defined? |
| Concurrency | Can concurrent requests cause race conditions, data corruption, or duplicate operations? Are locks, idempotency keys, or transaction boundaries needed? |
| Invalid input | Can malformed, malicious, or unexpected input bypass validation and cause undefined behavior? Are validation rules and rejection policies defined? |
| Boundary / limit | Is behavior defined under high load, large data volume, long-running execution, or resource exhaustion? Any resource leak risk? |
| Security | Is there unauthorized access, data leakage, or injection risk? Is auth enforced consistently at every entry point? |

If a plausible critical flaw is identified that the spec does not address → FAIL (needs_decision: needs user judgment on whether this is a design gap or intentional)

**Step 4 — Authoring Baseline verification (decision closure)**
Verify that the spec closes every implementation-affecting decision in `framework/spec_writing_guide.md` §9: the downstream executor must not be forced to choose. The §9 expression points are NOT judged here — the clarity check (Check 10) reads the spec and reports what is unclear, underspecified, or internally contradictory. This step verifies the decision closure that Check 10 does not test.

- **Input discipline:** the spec text is the ONLY input (main spec + non-exempt appendices). Do not fill spec gaps with implementation knowledge from this session — if the spec omits a decision, the gap is real and must be reported. Reading the implementation defeats this check's purpose: a spec that only makes sense with code knowledge forces the downstream executor to choose, which is exactly what the baseline forbids.

**Decision closure (the "must close" list):** Verify that the spec closes every implementation-affecting decision: which object owns a responsibility, which entry point starts the behavior, where state or durable truth lives, how ordered steps connect, how boundary failures are reported, what the result shape means, how acceptance proves the stated responsibility.

- For each of the seven decisions: can the downstream executor determine the answer from the spec alone, without making a choice?
  - For evidence-driven acceptance items (Step 2 waiver), the closure source is the evidence appendix: verify it records the item's behavior domain as directly readable behavioral truth (per `framework/spec_writing_guide.md` §3 `evidence_appendix_ref`), not only background, motivation, or patch notes.
  - The "how acceptance proves the stated responsibility" decision is covered by Check 5's acceptance quality review — here, only confirm the acceptance items exist and can prove the stated responsibility.
- If a decision is intentionally not made, the spec must state that boundary and explain why (per `framework/spec_writing_guide.md` §9, the "If a decision is intentionally not made" rule). An open decision without a stated boundary is a FAIL.
- Granularity: verify closure, not exhaustiveness — the spec must not be inflated into an implementation manual. Coverage obligations are limited to formal behavior domains (see Check 5a Step 2 extraction premise); narrative elaboration in the body does not add coverage obligations.

**FAIL:** a Step 1 goal-means flag (default severity P1; extraction artifact required), a Step 2 rationale gap, a Step 3 critical flaw; or any of the seven decisions is left open AND not explicitly bounded with a reason (actionable: record the decision / declare the boundary; needs_decision when recording it requires user input — Execution Rules "missing decision")

**Step 5 — Abstraction level & implementation agnosticism — owned by Check 7**

The Truth Ownership rules of `framework/spec_writing_guide.md` §14 (Anti-Patterns B, C, D: mandated internal mechanisms, brittle wall-clock sleeps, test-double leakage) are judged once, by Check 7 step 8 (shadow specification / over-specification). Check 2 does not re-judge them; Check 7's verdict and finding route apply.

**Step 6 — Verdict**
- PASS: goal-means aligned, per-item rationale documented (evidence-driven items waived per Step 2), all seven §9 decisions closed or explicitly bounded, and no critical flaws found
- FAIL: specific findings reported

**Check method:** Content reasoning + adversarial analysis + authoring baseline verification (the subagent makes active engineering judgments)

---

## Check 3 — Scope integrity

**Purpose:** The declared scope, non-goals, and boundaries must be clear and internally self-consistent.

**Execution steps:**

1. Is the unit's goal and responsibility scope clearly stated?
2. Are first-round non-goals and boundaries explicitly defined?
3. Are dependencies, rule bindings, and ownership boundaries explicit?
4. **Self-consistency check (main spec):**
   - Do the goals and described behaviors agree? (goal description scope matches behavior scope)
   - Are the boundaries respected by the behavior descriptions? (e.g., boundary is "client-side validation only" but behavior describes server-side logic)
   - Non-goal conflicts ("non-goal says not doing X but behavior describes X") are NOT judged here — that contradiction is owned by Check 2 Step 1's goal-means analysis, which reads the same declarations; judging it twice reports one defect as two findings.
5. **Appendix scope check:** Verify that appendix content does not exceed the unit's declared scope. If an appendix describes behavior belonging to a different unit's responsibility → FAIL (actionable: move content to the correct unit or declare scope expansion)
**PASS:** Scope is clear and self-consistent; no non-goal is violated; appendix content stays within unit scope

**FAIL:** Ambiguous scope, goal/non-goal contradiction, boundary violation, or out-of-scope appendix content (actionable)

**Check method:** Multi-field cross-reference (goal × non-goal × behaviors × appendix content)

---

## Check 4 — Evidence-driven vs design-driven consistency

**Purpose:** Verify consistency between `evidence_appendix_ref`, the evidence appendix, and the acceptance items. The waiver decision is per acceptance item: an item is evidence-driven when it references the evidence appendix in `affects.appendices`; otherwise it is design-driven. Mixed states are legal and expected during incremental replacement (see `framework/operations/adopt.md`). This check also detects zombie, orphan, and residual evidence states, all reported at **default severity P1** (blocking — promote must not proceed until resolved).

**Execution steps:**

1. **Per-item classification:**
   - If `evidence_appendix_ref` is PRESENT and not `none`:
     - Acceptance items whose `affects.appendices` references the evidence appendix → evidence-driven (rationale waiver applies, Check 2 Step 2)
     - Acceptance items that do NOT reference it → design-driven (rationale review applies)
   - If `evidence_appendix_ref` is ABSENT or `none`:
     - All items are design-driven (new concept or pure design change)
     - IF any acceptance item has verification_type == inspectable
       AND evidence_requirements includes old_code_deleted and no_remaining_refs:
       → This candidate is a replacement
       → Verify old code retirement separately (unit_verify_checklist Step 4)

2. **Zombie detection (default P1):** For each evidence-driven acceptance item (its `affects.appendices` references the evidence appendix), verify the evidence appendix contains a behavior-domain section corresponding to the item's behavior. If the item references the appendix but no corresponding section exists → FAIL (P1): stale reference — convert the item to design-driven (remove the evidence reference, add design rationale) or update the appendix.

3. **Orphan detection (default P1):** For each evidence appendix content section, verify an evidence-driven acceptance item whose behavior domain corresponds to the section exists (i.e. an item that references the evidence appendix and matches the section's behavior domain). If a section has no corresponding acceptance item → FAIL (P1): orphaned evidence — retire the section (delete it) or add a referencing item.

4. **Residual detection (default P1):** For each evidence-driven item whose behavior domain has been redesigned in the candidate (the spec body describes new or changed behavior for that domain), report FAIL (P1): the item must be converted to design-driven and the corresponding evidence section retired.

**PASS:** evidence_appendix_ref is consistent with the spec body; no zombie, orphan, or residual evidence states

**FAIL:** Contradiction found, or zombie/orphan/residual evidence state detected (P1, actionable)

**Check method:** evidence_appendix_ref × acceptance item attributes × evidence appendix content cross-reference

---

## Check 5 — Acceptance coverage & correctness

**Purpose:** The spec body and acceptance items must cover each other bidirectionally — every designed behavior has an item (5a forward coverage), and every design-driven item maps back to a designed behavior (5a orphan detection; evidence-driven and replacement/cleanup items excepted). Body and item content must match semantically and contain no internal contradictions (5b), and every acceptance item must be falsifiable, actionable, and carry contract substance beyond its description (5e).

**Execution steps:**

### Sub-check 5a — Coverage & item-set correspondence

**Purpose:** Every behavior domain in the spec body and appendices must have at least one corresponding acceptance item, every design-driven acceptance item must correspond to a designed behavior (orphan detection, step 7), and the item's surface fields must be consistent with the behavior type. Granularity baseline: behavior domains as defined in `framework/spec_writing_guide.md` §Acceptance Item Granularity — one item = one behavior domain with its full scenario set (happy path + error paths + boundary cases). Enhanced from the original forward coverage check to a bidirectional check.

**Execution steps:**

1. **Coverage input source:** A behavior is covered when any acceptance item describes it in its `description` (Given/When/Then scenarios) OR constrains it in its `pass_condition`. The coverage judgment input is the union of `description` and `pass_condition` — a behavior constraint that already appears in some item's `pass_condition` counts as covered, consistent with sub-check 5e (whose information-increment rule requires `pass_condition` to carry constraints beyond `description`).
2. **Extraction premise (shared with sub-check 5h):** Behavior-domain extraction targets only formal behavior declared in the spec body and appendices — a behavior subject (endpoint, function, state machine, or flow entry point) together with its behavior semantics. The subject must be externally observable per `framework/spec_writing_guide.md` §4: internal implementation detail (internal field names, internal field layouts, internal timing — including retry/backoff values, internal data structures and their operations, internal function behavior, configuration layout) is design expression, not a behavior domain source, and creates no coverage obligation — the same boundary step 6 and sub-check 5h apply on their surfaces. Non-constraining narrative (design discussion, illustrative examples, motivation, variant elaboration) is NOT a behavior domain source and does not create coverage obligations. Extract all behavior domains at the granularity defined in `framework/spec_writing_guide.md` §Acceptance Item Granularity: group behavior variants around one behavior subject into one domain (error paths, boundary cases, and state transitions of the same subject are scenarios of that domain, not separate domains); do not split scenarios of the same domain into separate coverage requirements.
3. For each behavior domain, verify at least one acceptance item covers it (using the union input from step 1)
4. For each covered domain, verify the item's `implementation_surface` and `verification_surface` are consistent with the behavior's nature (e.g., REST API behavior should have surface `api`, not `db`)
5. If a behavior domain has no acceptance item → flag (possible untested behavior)
6. **Appendix behavior coverage check:** Extract all behavior domains, API contracts, data type definitions, and state machine transitions from appendix files — for contract content, apply the external-visibility boundary of `framework/spec_writing_guide.md` §4 first: internal field names, internal field layouts, internal timing — including retry/backoff values, internal data structures and their operations, internal function behavior, and configuration layout are design expression, not contract content, and create no coverage obligation. For each extracted domain or contract, verify there is at least one acceptance item in the main spec covering it. If an appendix describes contract or behavior content that has no corresponding acceptance item → **FAIL (actionable)** — the acceptance item set is the complete formal behavior carrier (see `framework/spec_writing_guide.md` §4), and contract content without item coverage is invisible to the cross-unit consistency check of every dependent unit. If appendix content directly contradicts an acceptance item (e.g., appendix says "timeout: 30s", item says "respond within 5s") → FAIL (actionable)
7. **Orphan item detection (reverse check, FAIL):** For each design-driven acceptance item — an item whose `affects.appendices` does not reference the evidence appendix (Check 4 step 1) — verify the item's behavior subject (endpoint, function, state machine, or flow entry point, per the extraction premise in step 2) appears as a designed behavior somewhere in the candidate spec union: the main spec body or a non-evidence, non-exempt appendix. The subject may be designed as part of a larger flow; the test is subject presence, not narrative repetition of every contract element — the carrier obligation runs only one way, and contract elements are carried by the item itself under `framework/spec_writing_guide.md` §4. Exclusions: evidence-driven items are out of scope (their correspondence partner is the evidence appendix, enforced by Check 4); replacement/cleanup items are out of scope — `verification_type: inspectable` items whose `evidence_requirements` include `old_code_deleted` and `no_remaining_refs` are round-transitional verification requirements, not behavior declarations (the Check 4 step 1 replacement signal). If the subject appears in no designed behavior → **FAIL (P1, actionable):** retire the item together with its surviving narrative and appendix content (`framework/spec_writing_guide.md` §9 Cleanup obligation), or restore the behavior's design if it was dropped by mistake.
8. **Extraction evidence (required for every uncovered-domain and orphan-item FAIL):** Each uncovered-domain finding (steps 3, 5, and step 6's uncovered-content case) and each orphan-item finding (step 7) must carry an extraction artifact that makes the claim falsifiable:
   - **Source quote:** for an uncovered domain, the section heading and quoted text in the spec body or appendix that declares the behavior domain; for an orphan item, the item id and the quoted subject terms from its `description` / `pass_condition`
   - **Granularity judgment:** for an uncovered domain, why the quoted text is formal behavior (per the extraction premise in step 2, including the §4 external-visibility boundary applied to both body behavior domains and appendix contract content) rather than non-constraining narrative or internal design detail, and why its behavior variants form one domain (per the four granularity conditions in `framework/spec_writing_guide.md` §Acceptance Item Granularity) rather than scenarios of an already-covered domain; for an orphan item, why the quoted terms name a formal behavior subject of this unit (endpoint, function, state machine, or flow entry point) rather than a scenario of a designed domain or a dependency's behavior
   - **Absence claim:** for an uncovered domain, the covered surface checked (the union of every item's `description` and `pass_condition`, per step 1) and how the absence of any covering item was verified; for an orphan item, the surfaces checked (the main spec body and every non-evidence, non-exempt appendix) and how the absence of a designed behavior for that subject was verified
   A step-6 contradiction finding (appendix content contradicting an acceptance item) carries the two-sided quoted evidence step 6 itself requires — it is a conflict claim, not an uncovered-domain claim, and is not subject to this template.
   A finding without this artifact is not presented — the independent final synthesis re-verifies the artifact before classification and marks unfaithful claims suppressed (a subject actually mentioned in an item, or variants split out of a covered domain, or a designed behavior whose subject matches an allegedly orphaned item — see §Step 9 → Extraction re-verification).

**PASS:** All behavior domains (main spec + appendices) have corresponding items with appropriate surface fields; every design-driven item corresponds to a designed behavior

**FAIL:** Uncovered behavior domain or surface type mismatch (actionable); orphan design-driven item with no corresponding designed behavior (step 7, actionable); appendix contract/behavior content without item coverage (actionable); appendix-main spec contradiction (actionable); contract statement without carrier coverage (5h, actionable); item violating the item-substance rules (5e, actionable)

**Check method:** Spec body + appendices × acceptance item set bidirectional cross-reference (body → items for coverage; items → body for orphan detection)

---

### Sub-check 5b — Semantic consistency

**Purpose:** For each behavior–item pair, detect semantic contradictions between the spec body description and the item's `pass_condition`. This catches body edits that invalidate item content — whether from recent changes or historical drift.

**Execution steps:**

1. For each behavior in the spec body, identify its corresponding acceptance item(s)
2. Read both the body description text and the item's `pass_condition` text
3. Apply natural language reasoning to identify semantic contradictions:

| Contradiction type | Body says | Item says |
|---|---|---|
| Value conflict | "timeout: 30s" | "respond within 5s" |
| Behavior conflict | "login accepts email+password" | "return error when password provided" |
| Scope conflict | "supports OAuth and API Key" | "only validates API Key" |
| Direction conflict | "increment counter" | "decrement counter" |

4. For each contradiction, report with **exact quotes** from both sources and a reasoning statement
5. **Items × items (internal consistency):** group items sharing a `verification_surface`, and items sharing an `affects.files` entry; compare their `pass_condition` texts for contradictions — value conflicts (item A says "returns 201", item B says "200" for the same API), behavior conflicts (item A says "write requires auth", item B says "write is public"), numeric contradictions (one says <100ms latency, another <5s for the same operation), and logical contradictions (one says "enabled by default", another says "opt-in only"). Report each with quoted evidence from both items.

**PASS:** No contradictions found

**FAIL:** One or more contradictions found, with quoted evidence (actionable)

**Check method:** Spec body × item pass_condition, and item × item (by verification_surface and affects.files) — semantic cross-reference with quoted evidence

---

### Sub-check 5d — Description format (mechanized)

Testable items must use Gherkin-style Given/When/Then descriptions. The rule is enforced mechanically by `specflowctl validate candidate` Check 2 (`CheckAcceptanceItemSchema`): a testable item whose `description` lacks a Given…When…Then sequence, or that uses `.feature` file syntax, fails the mechanical pre-pass before any session launches. The session does not re-check description format.

### Sub-check 5e — Item substance

**Purpose:** Every acceptance item must be a concrete, testable, information-bearing statement: falsifiable, actionable, carrying contract substance, and adding value beyond its description.

**Execution steps (per item):**

1. **Falsifiability:** apply first-principles reasoning — "if the implementation were broken, would there be a way for this pass_condition to reveal it?" Identify a concrete, observable, and distinct failure scenario and write: "This item would FAIL if [specific code behavior or condition] occurs." No identifiable scenario → FAIL — Unfalsifiable (actionable: replace with a specific, measurable pass_condition). Pass conditions referring to external systems or runtime constraints that are not statically verifiable are recorded as CANNOT_DETERMINE in verify, not failed here.
2. **Actionability:** for `verification_type: testable` items, the `description` must contain enough detail to derive specific test scenarios — inputs or conditions, expected output or state change, and at least one boundary or edge case. A single vague sentence with no scenario breakdown → FAIL; a long but purely narrative description with no testable specifics → FAIL. Descriptions must not hardcode test doubles, mock runner names, or transient fixtures (`framework/spec_writing_guide.md` §14.2 Anti-Pattern C) → FAIL (actionable: restate using domain roles).
3. **Information increment:** compare `description` and `pass_condition` — the pass_condition must reference specific values, status codes, field names, error types, state transitions, timeouts, or behavior variants the description does not. A pass_condition semantically equivalent to or vaguer than the description → FAIL.
4. **Contract substance (S1–S5, `framework/spec_writing_guide.md` §7):**
   - **S1 — Contract element sufficiency:** description and/or pass_condition carries at least one concrete contract element (numeric constraint, HTTP status code, field/type/enum name, error code, protocol format, timing/consistency assumption); pure narration → FAIL.
   - **S2 — Specific-value obligation:** constraints use concrete values — `201` not `2xx`, `5s` not "fast", enumerated methods not "multiple methods" → FAIL otherwise.
   - **S3 — Scenario completeness:** the Gherkin scenario set includes the happy path plus at least one failure or boundary scenario (testable items); non-testable items carry at least one failure or edge condition in the pass_condition → FAIL otherwise.
   - **S4 — Information increment:** overlaps rule 3; a pure rephrase → FAIL.
   - **S5 — No template phrasing:** no content-free boilerplate ("behaves as expected", "processed correctly", "meets user expectations") → FAIL.
5. **Boundary:** the specific values S1/S2 require must be genuine Contract Anchors owned by this unit or public contracts exported by dependencies (`framework/spec_writing_guide.md` §14); private implementation details, arbitrary wall-clock sleep durations, or a collaborating unit's unexported fields do not satisfy contract substance — the Truth Ownership judgment itself is Check 7 step 8's.

**PASS:** All items are falsifiable, actionable, and carry contract substance beyond their descriptions

**FAIL:** One or more items violate any rule above (quote the violated rule and the offending text)

**Check method:** Per-item semantic assessment with quoted evidence; one item's violations are reported as one merged finding (see Per-item merge rule).

### Sub-check 5h — Contract statement carry-over (NEW)

**Purpose:** Contract statements in the spec body and non-evidence appendices must be carried by a formal behavior carrier (the acceptance item set or a protocol appendix, see `framework/spec_writing_guide.md` §4). The cross-unit check reads only carriers, so a contract that lives only in prose is invisible to every dependent unit. The obligation is scoped to the external-visibility boundary of `framework/spec_writing_guide.md` §4: only statements declaring the unit's externally-observable behavior are taxed — internal implementation detail is design expression and creates no carrier obligation.

**Execution steps:**

1. Extract **contract statements** from the spec body and every non-evidence, non-exempt appendix:
   - Apply the external-visibility boundary of `framework/spec_writing_guide.md` §4 first: a statement declaring the unit's externally-observable behavior — what a caller or dependent unit must read to use this unit safely (API surface, exchanged data formats, error codes, protocol semantics, timing/consistency guarantees) — is a contract statement; internal implementation detail (internal field names, internal field layouts, internal timing — including retry/backoff values, internal data structures and their operations, internal function behavior, configuration layout) is not, even when it states concrete values
   - Then classify each statement by type:
     - Numeric constraints (timeout, rate, limit, TTL values)
     - HTTP status codes and error codes
     - Field / type / enum names and data formats
     - Protocol names and formats
     - Timing / consistency assumptions (sync vs async, ordering guarantees, consistency expectations)
   - Non-constraining narrative (design discussion, illustrative examples, motivation) is NOT a contract statement — same judgment principle as sub-check 5a's extraction premise (see 5a Step 2)
2. For each contract statement, verify a carrier carries it at a **comparable granularity**:
   - The acceptance item set (an item's `description` or `pass_condition` states the same value, code, field name, format, or assumption), or
   - A protocol appendix (API contract, data type, error code, state machine section) states it
3. A contract statement with no carrier → FAIL (actionable: move the contract into an acceptance item or a protocol appendix)
4. **Extraction evidence (required for every FAIL finding):** Each uncarried-contract finding must carry an extraction artifact that makes the claim falsifiable:
   - **Source quote:** the section heading and quoted sentence in the spec body or appendix that states the contract
   - **External-visibility judgment:** why the statement declares externally-observable behavior (per the §4 external-visibility boundary) rather than internal design detail
   - **Absence claim:** the carrier surface checked (the acceptance item set and every protocol appendix) and how the absence of any carrier at comparable granularity was verified
   A finding without this artifact is not presented — the independent final synthesis re-verifies the artifact before classification and marks unfaithful claims suppressed (a value actually carried, or a statement reclassified as internal detail — see §Step 9 → Extraction re-verification).

**PASS:** Every contract statement in prose is carried by an item or protocol appendix

**FAIL:** One or more contract statements lack carrier coverage (actionable)

**Check method:** Body + non-evidence appendices × (acceptance item set ∪ protocol appendices) — contract-level cross-reference, quoted evidence per statement

---

## Check 6 — Affects-source validity

**Purpose:** Each acceptance item's `affects` declarations must be consistent with the spec's formal references. Evidence appendix content must be structurally sound and semantically meaningful.

**Execution steps:**

1. For each acceptance item, verify:

```
affects.rules:
  - Each rule must be either a global rule or listed in frontmatter rule_refs
  - Referencing a non-existent or undeclared rule → FAIL

affects.dependencies:
  - Each dependency must appear in frontmatter unit_refs
  - Referencing an undeclared dependency → FAIL

affects.files:
  - Each file path must point to an existing file in the project
  - Path should belong to this unit's ownership (if ownership is clearly defined)

affects.appendices:
  - Each appendix must exist and belong to this unit
  - The appendix frontmatter must declare the correct unit
```

2. If `evidence_appendix_ref` is not `none`:
```
- Read the referenced appendix file
- Its content must record actually observed implementation behavior
  (not only background, motivation, principles, or patch notes)
- If the appendix describes existing implementation behavior mixed with new design parts,
  it must clearly distinguish which parts are existing and which are new
- If content is only background or patch notes → FAIL (actionable)
- Each acceptance item that references the evidence appendix in `affects.appendices`
  must have a corresponding behavior-domain section in the appendix content;
  zombie/orphan/residual states are reported by Check 4 at default severity P1
```

3. **Appendix file path references:** For each non-exempt appendix, scan its content for code file path references (strings containing `/` and a source-code file extension). For each path found, verify it points to an existing file in the project. If any path does not exist → FAIL (actionable: update or remove the invalid path reference)

4. **Behavior participation (folded from Check 9):** for each declared implementation file (`implementation_surface` + `affects.files`), confirm the file participates in the declared behavior — read it and the item; shared implementation is permitted, and file overlap alone is never FAIL. A declared file with no relevant code → possible over-declared scope; undeclared relevant code is verify Step 3's finding, not this check's.

**PASS:** All affects declarations are valid, evidence appendix is semantically consistent, appendix file path references exist, and each declared implementation file participates in its item's behavior

**FAIL:** Reference inconsistency, appendix content contradicts declaration, or appendix references non-existent file paths (actionable)

**Check method:** affects.* × frontmatter refs cross-reference + appendix content semantic assessment

---

## Check 7 — Cross-unit consistency

**Purpose:** The candidate spec must not contradict related units (candidates and stables). Unacknowledged contract changes must be flagged.

**Execution steps:**

1. From `unit_refs`, get the list of dependency units (bare names, resolved candidate-first per Check 1)
2. For each dependency unit, read only its **formal behavior carriers** (see `framework/spec_writing_guide.md` §4):
   - The dependency unit's acceptance item set in the main spec (candidate first, then stable)
   - Its protocol appendices — files carrying API contracts, data type definitions, error codes, or state machine transitions
   Body prose and evidence appendices are not carriers: the no-contradiction assertion does not depend on them, and edits there must not stale dependent caches. Carrier completeness is guaranteed by the dependency unit's own validate (Check 5a fails contract content without item coverage), so reading the carriers reads the dependency unit's complete formal behavior.
3. Candidate spec takes priority — check the carriers for conflicting statements about shared protocols, data formats, or behavior; when the candidate names specific contract symbols (protocol names, field/type names, error codes), locate and verify those exact carrier regions first
4. Also read the stable spec's carriers and check whether this candidate changes any contract that the stable spec depends on — skip the stable contract check if the dependency's candidate spec already reflects the change
5. Specific checks:
```
   - Are API signatures compatible across all related units?
   - Are data formats (field names, types, enum values) consistent?
   - Is behavior semantics non-conflicting? (e.g., unit A assumes sync, unit B assumes async)
   - Does this candidate modify a contract that a dependency stable spec relies on?
     If yes, and the dependency's candidate spec does not already reflect the same change
       → the change must be explicitly declared in this candidate's spec body
       If not declared → FAIL (needs_decision: needs user confirmation on downstream impact)
```
6. **Protocol appendix cross-unit check:** Include the dependency unit's protocol appendix contracts in the cross-unit comparison (they are carriers). If a protocol appendix defines a contract, data format, or protocol that conflicts with another unit's spec → FAIL (actionable: resolve the cross-unit inconsistency)
7. **Shared agreements and dependency declarations (folded from Check 9):** shared behavior, data meaning, and boundary agreements must have one authoritative source — shared constraints live in rules consumed through `rule_refs`, not restated independently in several specs. Keep actual behavior dependencies in `unit_refs`/`affects.dependencies`; never infer a dependency from a shared file alone.
8. **Cross-Unit Private Implementation Leakage (Anti-Shadowing Rule, FAIL):** This step is the single owner of the Truth Ownership rules (`framework/spec_writing_guide.md` §14.1 and §14.2); Check 2 Step 5 and Check 5e's boundary reference it. Enforce the Truth Ownership Visibility Law (`framework/spec_writing_guide.md` §14.1 and §14.2 Anti-Pattern A). A non-owner unit must never enumerate volatile, unexported, or private internal parameters/fields of collaborating units.
   - For every attribute, parameter, field name, or payload structure cited by the candidate spec regarding a dependency unit, verify that it resolves to one of:
     - **Public Contract Anchor:** An exported public contract symbol or explicit schema property declared in the dependency's formal behavior carriers (acceptance item set or protocol appendix), such as `contracts.SpanAttr*`, standard exported protocol events, or public REST/gRPC response keys.
     - **Behavioral Specification:** A behavioral description that does not hardcode unexported parameter names (e.g. "records delegation goal and child execution identifier" instead of literal private parameter names `goal, role, child_run_id`).
   - **Transitive Penetration Check:** If the candidate spec cites internal fields or types of an indirect dependency (a unit not declared in `unit_refs`), verify whether an explicit contract path exists. Citing internal fields of undeclared transitives without an exported contract anchor is a boundary violation.
   - If a spec mirrors or hardcodes unexported, private, or volatile internal fields of another unit:
     - **FAIL (P1: Shadow specification)** (actionable: replace private field enumerations with public contract anchors or behavioral specifications).

**Evidence:** read the carrier regions the judgment depends on — the dependency unit's acceptance item set and the protocol appendix files — from the session's read refs. Reports carry no dependency declarations; the tooling records the run's whole input surface (see §Step 8).

**PASS:** No contradictions across related units; acknowledged contract changes are declared; no shadow specifications found

**FAIL:** Contradiction found (actionable), shadow specification / private implementation leakage found (actionable), or unacknowledged contract breakage (needs_decision)

**Check method:** Candidate × related candidates × related stables three-way cross-reference over the formal behavior carriers (acceptance item sets + protocol appendices).

---

## Check 8 — Constraint alignment

**Purpose:** The candidate design must not violate global constraints or bound rules.

**Execution steps:**

1. Read the stable global rule set (`docs/specs/rules/stable/g_rule_*.md`) and each bound rule listed in `rule_refs`. Stable global rules apply to every current-layer unit by default and are not repeated in `rule_refs` (see `framework/spec_writing_guide.md` §5). Execute the two framework dependency prohibitions:
   - **Circular dependency prohibition** — run the mechanical check first (`specflowctl deps --unit <name>` / `deps@{unit}`, or `specflowctl validate candidate`, whose Check 7 fails every unit on a cycle): the mechanical verdict is authoritative for the FULL dependency graph — do not re-derive the whole graph independently. The session still verifies its own unit's declared edges: read this unit's `unit_refs` (required for the cross-unit work below anyway) and confirm each edge matches the graph the tool reports — a mismatch is a tool-input problem to report, not a judgment call. A cycle (A depends on B while B depends on A, directly or transitively) is a violation; analyze and present cycle-resolution guidance only when the tool reports a cycle. `deps@all` / `deps@{rule}` reproduce the graph, cycle members, and promotion order (see `framework/verification_scope.md` §Dependency Analysis). Graph build failures are fail-closed: an unreadable unit spec (permission, corruption) blocks the whole graph, so every unit FAILs Check 7 with the failing file named in the Details — repair the file and re-run validate; `deps@all` reproduces the failure.
   - **Layer-order prohibition** — for each applicable rule that records a repository layer order, resolve the order from that rule's recording and each unit's declared architecture layer from its spec truth (architecture section or design decision records); a `unit_refs` edge from a lower-layer unit to a higher-layer unit is a violation. Units whose spec records no architecture layer are not judged by this prohibition.
2. Check the candidate design against each global rule and each bound rule:
```
   - Is every "must not" prohibition respected?
   - Is every "must" requirement satisfied?
```
3. **Cycle resolution guidance (when the dependency graph contains a cycle):** analyze the cycle's nature — a shared contract region repeated across the cycle members' acceptance items should be extracted into a rule (making the dependencies star-shaped), or the unit boundaries should be re-drawn so the coupled contract lives in one unit. Present the analysis to the user; the user decides the resolution, and the fix is applied only after explicit approval. A cycle is never resolvable by promote ordering — promote is per-unit and the cycle members mutually block each other's acceptance items.
4. **Rule exception re-evaluation:** Read the candidate spec's frontmatter `rule_exceptions` field (see `framework/spec_writing_guide.md` §3). For every recorded exception, first verify its reference validity, then re-evaluate whether the exception still holds against the current implementation and the current rule content:
   - Referenced rule is neither a stable global rule nor a bound rule listed in this unit's `rule_refs`, or the reason is missing → FAIL (actionable: correct or remove the invalid exception entry)
   - Exception no longer justified (architecture was rewritten, rule changed, or the reason expired) → FAIL (actionable: report the exception for removal; the removal is applied only after user approval)
   - Exception still justified → keep it and state the re-examination verdict in this check's reason
5. **Appendix constraint check:** Include appendix design descriptions, API contracts, and behavior definitions in the constraint and bound rule checking. If appendix content describes behavior that violates a global constraint or bound rule → FAIL (actionable: align appendix content with constraints)

**PASS:** Candidate (main spec + appendices) is compatible with all constraints and bound rules; all recorded rule exceptions are still justified

**FAIL:** Constraint or rule violation found in main spec or appendices (actionable)

**Check method:** Candidate × stable global rule set × bound rules three-way cross-reference

---

## Check 9 — File associations (mechanical)

**Purpose:** Report the mechanical surface-association audit for this unit's declarations. The semantic halves of the former Check 9 live with their owning checks: whether each declaration participates in its item's behavior is Check 6 step 4; shared-agreement single-source consistency and `unit_refs` accuracy are Check 7 step 7.

**Execution steps:**

1. Run `specflowctl surfaces` and confirm the mechanical `Surface associations` audit for the target passed. Read current and stable associations, directory expansion, and each unit's declared scope: every declared file resolves, and the derived associations agree with the declarations. Invalid paths are handled by the anchor checks.
2. Report the mechanical verdict: PASS when the audit passes and neither Check 6 nor Check 7 reported an association contradiction; FAIL when the audit reveals an anomaly (an unresolvable or inconsistent association), with the file and the contradiction named — the semantic judgment is routed to Check 6 or Check 7.

**PASS:** The association audit passes and no owning check reported an association contradiction.

**FAIL:** An association anomaly the audit reveals — P1 with the existing evidence fields. File overlap alone is never FAIL.

**Check method:** Mechanical association audit + consistency with the owning checks. See `framework/shared_judgments.md`.
## Check 10 — Clarity

**Purpose:** Read the unit spec and report what is unclear, underspecified, or internally contradictory. This is a single independent session, judged like every other validate check (report `PASS | WARNING | FAIL` plus the standard finding block) — there is no reader/verifier split and no closed-book reconstruction. It judges the Reader Contract (`framework/spec_writing_guide.md` §9) directly: whether the spec's human-readable part states the design clearly enough that its readers can understand it without guessing, and whether any statement conflicts with another.

**Reading scope:** the unit's complete spec — the main spec and every non-exempt appendix (the same union as every other check). The check reads it directly; it reconstructs nothing.

**Execution steps:**

1. Read the complete unit spec union (see Prerequisite).
2. For each behavior, mechanism, or decision the spec states, judge whether a reader without code access can understand it from the text alone:
   - **Unclear** — the text names a mechanism, field, or step without saying what it does or how it connects to the rest.
   - **Underspecified (expression level only)** — the text hides an already-made decision or leaves a reader unable to act on a decision the spec did make (e.g., the governing statement sits where the reader was never pointed, or two passages imply different answers to the same question). A decision that is genuinely OPEN with no stated boundary is NOT reported here — decision closure is owned by Check 2 Step 4 (`framework/spec_writing_guide.md` §9 must-close list). When both readings could apply, Check 2 owns the finding; this check does not duplicate it.
   - **Internally contradictory** — two statements disagree (a value, an error code, a flow direction, or a contract stated differently in two places).
3. Grade each defect: an internal contradiction that leaves the downstream executor unable to determine the design is a P0/P1 finding; a localized wording or presentation gap that does not change meaning is a WARNING. Open decisions with no stated boundary are never graded here — Check 2 Step 4 owns them.
4. A FAIL must carry at least one P0/P1 finding in the standard finding block: quote the unclear or conflicting text, name the concrete subject, and state the concrete repair (`fix:`) or the decision the user must supply (`decision:`).

**PASS:** The spec's human-readable part is clear, no statement contradicts another, and every stated decision is readable without guessing.

**WARNING:** Localized clarity gaps (wording, presentation) that do not change the stated design or block the reader.

**FAIL:** An internal contradiction, or a passage a reader cannot understand — reported as a P0/P1 finding (actionable when a concrete repair exists; needs_decision when the intent must come from the user).

**Check method:** Direct semantic reading of the spec by one independent read-only session — no reconstruction and no second session; the verdict and finding block follow the standard checks format.

### Scope and lifecycle

- **Delta/repair:** check 10's dependency evidence is the spec sections (and appendices) the clarity judgment read. An edit inside a declared section or appendix re-runs check 10; rule and dependency changes carry it over like any other check.
- **Stable-only targets:** the check runs against the stable main spec and appendices like every other check.
- **Empty acceptance item set:** runs normally — the clarity judgment reads the spec text, not the item set; validate still plans the run so Check 2 can report the empty set.
- **Targeted runs:** `validate@{unit}:clarity` (or `:check-10`) executes the same single-session clarity check directly in the main agent session, without a coverage run or cache write; a P0/P1 finding must first be persisted with `gate-invalidate --check 10`.

---

## Step 9 — Write validate cache (tooling finalize)

When relationships are assigned or findings exist, after all local check sessions complete the independent final session re-verifies the extraction artifacts of Check 2 Step 1 and Check 5 FAIL findings (sub-check 5a step 8 / sub-check 5h step 4), checks only the assigned validate relationships without repeating local checks, and supplies the dispositions and relationship results the tooling's status closure consumes (the effective check-status map is tool-derived — the report carries no status lines). `gate-finalize` then decides the cache mechanically from that accepted synthesis:

==ATOM_BEGIN:cache_evidence_path_forms==
**Recording cache evidence — tool-generated:** gate cache entries are never agent-declared. `gate-finalize` records one entry per file of the run's input surface (the union of every coverage key's read refs and the cross-synthesis surface), each carrying the whole-file `hash` and the ordered content-defined chunk sequence of the current bytes, plus a per-check marker (check key + lens; failure status on failure records). Reports carry no `Dependency scope` lines; there is nothing for a reviewer to declare and nothing to validate against `read_refs`.

Spec objects that are resolved by name (a dependency unit's main spec or appendix, a rule file) are recorded as **logical references** instead of physical paths — `unit:{name}` for a unit main spec, `unit:{name}:appendix:{file}` for a unit protocol appendix (the full appendix file base name without `.md`, e.g. `unit:auth:appendix:unit_auth_account_token_claims`), `rule:{id}` for a rule file. The run's own target files (the unit's own main spec and appendices; for a rule target, the candidate rule file and its stable sibling) and code files keep physical paths. A logical reference resolves at freshness time to the current-layer file (candidate first, stable fallback), so promoting the referenced unit or rule does not stale a cache whose recorded content is unchanged (see `framework/validation_cache.md` §Logical References).
==ATOM_END:cache_evidence_path_forms==

### Extraction re-verification (Check 2 Step 1, Check 5)

Step 1 goal-means findings (Check 2), uncovered-domain and orphan-item findings (sub-check 5a, step 8 artifact), and uncarried-contract findings (sub-check 5h step 4) carry an extraction artifact. Before classification (cross synthesis), the independent final synthesis re-verifies each artifact with deterministic checks:

- Sub-agent claims a Check 2 Step 1 flag holds ("the flagged behavior serves no stated goal", "the described behaviors cannot meet the goal", "the behavior violates a non-goal", or "a smaller design surface would achieve the same goal") → re-read the goal or non-goal declaration and the flagged behavior or design surface; a quote that does not establish the claimed relationship — including a proportionality claim resting on preference rather than an establishable smaller surface — drops the finding
- Sub-agent claims "no item covers behavior subject X" → re-read the item set (the union of every item's `description` and `pass_condition`) and confirm X's quoted subject terms are really absent; a subject actually mentioned, or behavior variants split out of a covered domain (granularity violation), drops the finding
- Sub-agent claims "no designed behavior matches item X" (orphan item) → re-read the spec union (main spec body and every non-evidence, non-exempt appendix) and confirm X's quoted subject terms are really absent as a designed behavior; a designed behavior whose subject matches, or terms naming a scenario of a designed domain, drops the finding
- Sub-agent claims "no carrier states contract value Y" → grep the item set and the protocol appendices for the quoted value; a value actually carried at comparable granularity drops the finding
- Sub-agent claims "Z is a behavior domain" (5a step 2), "Z is a contract statement" (5h), or "Z is appendix contract content requiring acceptance coverage" (5a step 6) → check the classification against the external-visibility boundary of `framework/spec_writing_guide.md` §4 (externally-observable behavior vs internal design detail); content reclassified as internal detail drops the finding

Re-verification failure → the cross result marks the finding `suppressed`; the affected check's tool-derived effective status is PASS if no retained finding supports FAIL. This runs before classification because a post-execution check re-run cannot detect a wrong-direction fix. Every suppression remains visible in the cross audit artifact with its location and reason; only retained findings enter the generated user report and counts.

- **If all checks PASS (after re-verification):** the coverage run writes the validate cache per `framework/validation_cache.md` format:
  - `gate-finalize` creates `docs/specs/meta/validation/unit/{name}/` as needed
  - Collect dependency evidence for every file read during validation, including:
    - Main spec file
    - Every non-exempt appendix file
    - All referenced files (unit_refs, rule_refs, affects.files are already included)
  - `gate-finalize` assembles the cache from the accepted reports and the run input surface: one entry per input-surface file (whole-file hash + ordered chunk sequence) and one marker per executed check (check key = the agent check number; see `framework/validation_cache.md` §Format).
  - The `gate-finalize` write produces `validate_result.md` with `result: pass`, `target: candidate`, `mode: full`, and the complete input-surface evidence (whole-file hash + ordered chunk sequence per file).
  - Targeted runs (`:check-{n}` / `:{keyword}`) never write a cache, and a targeted run that FAILs deletes a pass cache (a failure record is kept — it is already blocking and is the recovery baseline) — any FAIL at any granularity means promote must not proceed — see `framework/validation_cache.md`

- **If any FAIL remains (after re-verification, full run, candidate target):** `gate-finalize` writes a failure record (`result: fail` + `blocking: true`, `mode: full`, `basis: full`, severity counts, findings body, and the per-check `status` map — `pass`/`fail` for every executed check; full runs re-execute local checks and may carry unchanged relationship judgments) — the failure-recovery baseline for `revalidate@{unit}`. Promote must not proceed. Proceed to Present Findings. (A stable-only full FAIL writes the same record shape — see §Stable-only mode.) After the findings are resolved, `revalidate@{unit}` (repair) re-checks the failed checks, persisted invalidated checks, newly affected checks, current keys the baseline never declared, and explicit `--rerun` overrides, then carries the passed checks over — each carried `pass` was executed as a complete independent judgment and its recorded content is unchanged (see §Delta re-run and §Failure handling by gate role in `framework/validation_cache.md`). During the fix iteration, targeted re-checks (`validate@{unit}:check-{n}` / `:keyword`) remain the lightweight local option.

### Delta re-run (revalidate@{unit})

Candidate targets, plus stable-only targets with a usable baseline — a delta re-run against a stable-only target whose confirmation cache is MISSING reports the missing-baseline message — "No usable confirmation baseline. Run the full `{command}@{target}` (confirmation check) first, or `specflowctl fork {--unit|--rule} {name}` to start a new round." — and stops (see `framework/verification_scope.md` §Delta Runs → Layer applicability). Triggered by the user when the cache is STALE or BLOCKED (a failure record). Follow §Delta Runs in `framework/verification_scope.md`:

1. **Preconditions** — the existing cache must have `mode: full` and one of the two baselines: `result: pass` (stale-cache recovery) or `result: fail` + `blocking: true` (failure-record recovery, §Failure recovery — the record written by a candidate full-run FAIL or by a delta FAIL). A MISSING cache has no usable baseline — run the full command instead.
2. **Plan (change review)** — run `specflowctl gate-plan --gate validate (--unit {name} | --rule {id}) --target {candidate|stable} --mode delta` (or `--mode repair` for a failure record). The planner computes the mechanically detected change set (ordered bidirectional chunk diff over the recorded input surface; new physical inputs join as added files), schedules one `review` session carrying the change set and the standing conclusions, and mechanically forces invalidated verify records, failed and persisted-invalidated checks, current keys the baseline never declared, and explicit `--rerun` overrides. It reports the change set, the forced keys, the carry candidates, and any degradation or full-scope statement. After a targeted P0/P1, the coordinator runs `gate-invalidate` immediately; the planner reads that state automatically and never carries the contradicted judgment. A cache without the required chunk evidence is not current-format and is treated as no baseline — the planner refuses the delta/repair and directs a full run. A failure record whose per-check status map is absent, invalid, or inconsistent with its judgment baseline covers the full scope. The review decides the remaining scope; the executor does not re-derive it.
3. **Review and execute** — generate the review mission with `specflowctl gate-mission --run <run_id> --keys review --format prompt` and submit its report with `specflowctl gate-submit --run <run_id> --session review --keys review --report PATH`. `Review result: accept` keeps the planned carry set; `Review result: recheck` with `Recheck: <keys>` adds those conclusions' coverage keys to the run (re-mission and execute them; each re-run session uses the standard checklist with the change set as context); `Review result: escalate-full` abandons the run — start a full run instead. Then execute the planned coverage keys (protocol: `framework/verification_scope.md` §Coverage Model), submitting each re-run report with `specflowctl gate-submit --run <run_id> --session <session_id> --keys <keys> --report PATH`. Any P0/P1 finding during the re-run makes the finalize write a **failure record** (`result: fail`, `blocking: true`, severity counts, findings body, `mode: full`, `basis: delta`, and the per-check `status` map — `fail`/`pass` for the re-run checks, `carried` for the carried-over ones, derived by `gate-finalize` from the accepted session verdicts). Stop, present findings (same as full FAIL — Check 2 Step 1 and 5a/5h findings re-verified per §Step 9 → Extraction re-verification before presentation).
4. **On PASS** — `gate-finalize` rewrites `validate_result.md` with `mode: full`, `basis: delta` (or `basis: repair` when recovering from a failure record), `target: candidate` (stable-only target: `target: stable`), a fresh `timestamp`, the recorded review (`reviewed_change_set`, `review_result`, `review_session`, `review_recheck` when named), and a **complete** `files` list: fresh `hash` + `chunks` evidence for the re-run paths and the plan-time snapshot's evidence for the carried-over ones, merged by the tooling. Logical references (`unit:{name}` / `unit:{name}:appendix:{file}` / `rule:{id}`) are carried over the same way.

---

## Present Findings

Advisory findings (Check 1 step 3 hygiene WARNING) are presented for awareness only — they enter neither the batch group nor the decision group, need no decision, and do not block the flow. They are presented on their check line's reason even when all checks PASS. Advisory findings are never emitted as bracketed findings — validate grades findings P0/P1 only, and a retained P2/P3 finding rejects the run's synthesis at `gate-finalize`.

### Batch classification (validate)

When FAIL items exist, the main agent classifies each finding into a **batch group** or a **decision group** before presenting. Classification uses each FAIL's resolution type (actionable / needs_decision) and the check definition — it adds no new analysis; for batch group candidates it only runs lightweight assertion re-verification (see Assertion re-verification below).

**Batch group eligibility:** A finding enters the batch group only if its fix is fully determined by an objective standard — no interpretation of design intent is required. The batch group is limited to these fix types:
- Check 1: missing required frontmatter fields (standard: the required fields list)
- Check 1: unit_refs / rule_refs / appendix references to non-existent files (standard: file existence)
- Check 1: appendix path or naming not following the convention (standard: the path convention)

All other FAIL findings — including 5e item-substance rewrites, Checks 2/3/4/6/7/8, and every needs_decision item — go to the decision group. **needs_decision items always go to the decision group.**

No activation threshold: findings are aggregated at check level rather than presented flat per item, so splitting out the batch group adds constant cost and always reduces the decisions the user must make — there is no over-splitting scenario. The batch group is inherently limited by the fix-type list above.

==ATOM_BEGIN:batch_findings_mechanism==
**Assertion re-verification:** After eligibility passes, the main agent re-verifies each batch group candidate's core assertion with 1-2 deterministic checks:
- Sub-agent claims "X is missing" → confirm X is really absent from the cited location (read the file, check existence, or grep as appropriate)
- Sub-agent claims "the cited source states X explicitly" → read the cited line, confirm X is really written there, without vague wording ("or", "suggested", alternatives)
- Sub-agent claims "the correct pattern exists elsewhere" → confirm that reference really exists

Re-verification failure → item moves to the decision group. This runs before execution because a post-execution check re-run cannot detect a wrong-direction fix — once the documents are made consistent, the re-run passes and the error is cemented.

**Execution boundary:** Classification is presentation only — it is not an authorization to act. Nothing is implemented until the user explicitly agrees to the whole batch group. The user may approve the batch, release it without changes, or move individual items out to the decision group. Before confirming, the user may ask to expand any batch item (full analysis plus on-the-spot re-check); expanded items move to the decision group and are decided individually.

**Batch group presentation note:** When batch grouping is active, add: "This grouping is a classification suggestion only — nothing is applied until you confirm. Each item shows its judgment basis; ask to expand any item if in doubt — expanded items move to the decision group."
==ATOM_END:batch_findings_mechanism==

After classification, present the findings (§Summary format) and wait for the user's decision per HARD RULE 3a:
- **Batch group:** one decision on the whole group per §Batch classification.
- **Decision group:** present each finding with its resolution type (actionable / needs_decision) and wait for the user's decision per HARD RULE 3a. Do not offer a structured resolution menu.

### Severity handling

validate grades findings P0/P1. Each finding's severity is assigned by the session that raises it; the final synthesis (when the run has relationships to check or findings) may raise the canonical severity of a retained or merged finding conservatively — never lower it — and counts and blocking derive from those canonical severities. A P0 grade is a judgment-based assignment: the session that raises it must read the impact surface the grade depends on (the downstream consumer, dependent unit, or the section/appendix the claim relies on) and establish the boundary in `framework/severity_policy.md` §9. The contract-decided P1 default needs no boundary check. Targeted runs apply the same grading discipline but do not create run state.

### Summary format

Present the findings using the unified report skeleton (§Output Format). The header, `Blocking promote`, key counts, and `Next step` follow the skeleton; the Findings section is command-specific and defined here.

```
────────────────────────────────────────────
validate@{unit} · full · candidate   # targeted runs: validate@{unit} · targeted (user requested: {keyword}) · candidate
Result: FAIL
Blocking promote: yes
Key counts: Findings: N (P0: a | P1: b | P2: c | P3: d)
────────────────────────────────────────────
Findings:
  Batch group (N items) — fix fully determined by an objective standard:
    - [{severity}] {location} — {issue} (actionable, based on: {standard reference})
      problem: {one-sentence statement naming the concrete subject}
      evidence:
        - {verbatim quoted source content} — {source}
      impact: {what goes wrong if unresolved}
      fix: {concrete repair action}
    ...
  Decision group (M items) — need confirmation:
    1. [{severity}] {location} — {issue} (actionable | needs_decision)
      problem: ...
      evidence:
        - ...
      impact: ...
      fix: {repair action}   # actionable
      # or
      decision: {the question the user must answer}   # needs_decision
      options:
        - ...
    ...
────────────────────────────────────────────
{body — the executed check lines, one per check; Failed checks: N and Advisory findings: K shown in the body}
────────────────────────────────────────────
Next step: {actionable (finding_open) → "Resolve the findings, then re-run `validate@{unit}:check-{n}` to confirm"; needs_decision → "awaiting your decision on {item}"}
────────────────────────────────────────────
```

`Findings` (N) equals the sum of batch group items and decision group items.

Suppressed findings from the extraction re-verification (Check 2 Step 1, Check 5) are dispositioned in the cross report's audit artifact (see Step 9 → Extraction re-verification) and do not appear in the report or its counts.

When no finding qualifies for the batch group, present flat:

```
Findings:
  [{severity}] {location} — {issue} (actionable | needs_decision)
    problem: ...
    evidence:
      - ...
    impact: ...
    fix: ...
  ...
```

### Validate-specific notes

- **Re-validation rule:** After any fix is applied, the agent must NOT re-run validate automatically. Executing quality-gate commands is user-triggered only (see HARD RULE 2 in `framework/concepts.md`). The agent guides the user to a targeted re-check with the concrete command and waits for the user to trigger it. Affected-check mapping: acceptance item edits (any field) affect the Check 5 family — suggest `validate@{unit}:check-5`, since every sub-check reads item fields; `affects.*` edits additionally affect Check 6 — suggest `validate@{unit}:check-5` plus `validate@{unit}:check-6`; edits to spec body prose or appendices affect 5a/5b — suggest `validate@{unit}:check-5`. Example suggestion (`fixed_pending_recheck` — only after the fix was actually written): "Fixes applied; re-run `validate@{unit}:check-{n}` to confirm. Shall I run it?" (Targeted re-checks never write a cache — only a user-triggered `validate@{unit}` full run restores the cache.) Until a re-run is triggered, do NOT write a pass cache and do NOT claim the fix is verified — report "fixed, pending re-confirmation" (`fixed_pending_recheck`). When a re-run is triggered by the user, the final validate result is based on the re-run, not on the pre-fix snapshot. Findings from the pre-fix snapshot that are no longer reproducible on the re-run are dropped, not carried forward as still-open. A finding confirmed resolved by the re-check is reported as `verified` ("Re-check passed.") — this confirms the fix, not cache freshness (a targeted re-check never writes a cache). When a re-run changes an earlier finding, inform the user: "Re-validated affected checks after the fix. [finding] no longer holds. Remaining findings: ..."
- **needs_decision items** (resolution_type: needs_decision) require user input — skip to next finding without suggesting a fix.

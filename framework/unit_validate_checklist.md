# Validate Checklist

## Overview

When an agent executes `validate@{unit}`, it uses the 10 checks defined here. The trigger route in `framework/concepts.md` loads this file at validate time, not proactively.

## Target-layer applicability

Use the planned `Target layer` (`candidate` or `stable`) shown in the gate mission when applying this checklist. For a targeted run without a gate mission, use the layer resolved by the command: candidate when present, otherwise stable. All 10 checks apply to both layers. The Prerequisite reads the main spec and the appendices from that layer, and the Check 1 mechanical pre-pass is candidate-only: a stable-only target runs no mechanical pre-pass and its Check 1 is the agent-judged structural residue alone. A stable confirmation checks accepted content; it does not require a candidate file.

## Prerequisite — Read all unit files

Before executing any check, the agent must read the complete unit content. `{layer}` is the resolved target layer (`candidate` or `stable`) per §Target-layer applicability:

1. Read the main spec: `docs/specs/units/{layer}/unit_{unit}.md`
2. Glob all appendix files in the resolved layer: `docs/specs/units/{layer}/appendix/unit_{unit}_*.md`
3. For each appendix, read its frontmatter and confirm appendix ownership by its `unit` field.
4. Read the content of every appendix file.

The unit's complete spec is the union of the main spec and all appendix files. All checks that follow operate on this union.

### Promise-model checks

The unit-spec shape and promise contract are defined in
`framework/core/promise_model.md`. Unit validate enforces the mechanical
checks; `specflowctl validate promise-model --unit <name>` runs them
standalone. The promise standards are:

1. **Shape** — exactly one `## Design Narrative`, one `## Promises`, and one
   `## Acceptance Items` section, in that order; no other `##` section, and no
   `###` subheadings inside `## Promises` or `## Acceptance Items`.
2. **Promise schema** — stable unit-owned ID (`<unit>.<name>`), summary,
   and a non-empty normative `contract` built from the five contract blocks
   (`operations`, `data`, `occurrences`, `state_machine`, `invariants`); no
   code paths, verification methods, evidence, `verified_by`, or consumer
   lists; `depends_on` entries reference another unit's promise, resolve, and
   form no cycle — neither a promise cycle nor a unit dependency cycle (two
   units each having a promise that depends on a promise of the other).
3. **Coverage** — every promise is covered by at least one acceptance item;
   every contract-verifying item references a promise owned by the same unit;
   every non-contract item declares `non_promise: true` with a reason.
4. **Narrative support** — every promise is grounded in the design narrative;
   a contract statement in the narrative that no promise carries is a finding.
5. **Cross-unit consistency** — consumers depend on provider promises through
   the consumer promise's `depends_on`; a consumer item never verifies a
   foreign promise.
6. **Appendix boundary** — an appendix carries explanatory detail, examples,
   or evidence; normative contract content outside a promise is a finding.
7. **Granularity** — one promise carries exactly one behavior subject (one
   endpoint or callable surface, one event, one exchanged data schema, one
   state machine, or one standing property); a promise that bundles unrelated
   subjects is a finding, while several contract blocks describing one subject
   are facets of that subject; a consumer's `depends_on` names the narrowest
   promise that carries what the consumer reads.
8. **Item-proof consistency** — every contract-verifying item's `description`
   and `pass_condition` are consistent with the promise's `contract`: the item
   does not declare a different status, field, error, protocol, timing,
   interface or function signature, state, event, or invariant; it states one
   specific check of that promise, not a second contract.

Standards 1–3 are mechanical: `specflowctl validate candidate` runs them as
Check 15 (and `validate promise-model` runs the same checks standalone).
Standards 4 and 7 are reported by Check 5h, standard 5 by Check 7, standard 6
by Check 5a step 6, and standard 8 by Check 5b. They are not a separate report
key; a finding is reported under its owning check.

## Mode Selection

**Mechanical pre-pass:** for a candidate target, every full and delta `validate@{unit}` run starts with `specflowctl validate candidate --unit {name}` (see Check 1). A mechanical FAIL stops the run — fix and re-run the tool before planning; no gate run is planned and no session launches until it passes. A stable-only confirmation has no mechanical pre-pass — the tool checks the candidate layer only — so the Check 1 residual judgments stay agent-verified (see §Target-layer applicability).

| Trigger | Mode | What to execute |
|---------|------|-----------------|
| `validate@{unit}` | full | All 10 checks, grouped into 5 coverage keys by read surface: `structural`, `design`, `acceptance`, `dependencies`, `clarity`. Quality checks are holistic — always runs full. When relationships are assigned or the primary pass produced findings, the final session runs before finalize. |
| `validate@{unit}:check-{n}` | targeted | Single check `{n}` only. User explicitly chooses focus. Does not write a cache. |
| `validate@{unit}:{keyword}` | targeted | Match keyword to check name (e.g., "design" → Check 2, "scope" → Check 3). User explicitly chooses focus. Does not write a cache. |

**Keyword domain:** validate keywords resolve to check names — `structure` (Check 1), `design` (Check 2), `scope` (Check 3), `evidence` (Check 4), `acceptance`/`coverage` (Check 5), `affects` (Check 6), `cross-unit` (Check 7), `constraint` (Check 8), `ownership`/`surface` (Check 9), `clarity`/`clear` (Check 10). A keyword matching no check name is a no-match — ask the user for clarification.

**Output:** Targeted runs report only the executed check(s) and note "This was a targeted check — no complete cache was written. Run `validate@{unit}` for a complete validation." A targeted P0/P1 must first be persisted with `gate-invalidate --check {check key}`.

### Stable-only mode

When no candidate spec exists (validate against stable), run the same 10 checks against the **stable** content using §Target-layer applicability:

1. Read the stable main spec: `docs/specs/units/stable/unit_{unit}.md`
2. Glob all stable appendix files: `docs/specs/units/stable/appendix/unit_{unit}_*.md`, read every appendix
3. Run all 10 checks against the stable content — the Check 1 mechanical pre-pass does not run for a stable-only target (see §Target-layer applicability), and Checks 6/7/8/9 are the live part: referenced files, dependency-unit contracts (Check 7 resolves provider promises in the published stable layer), rules, and declared surfaces may have changed since promote, so the stable content may no longer hold (e.g. a new rule now prohibits something the stable design does, or another unit now declares a file this surface also declares)
4. **PASS** → `gate-finalize` writes the validate cache with `target: stable` (confirmation state consumed by `fresh@stable`; `mode: full`, whole-file hash + chunk evidence; same coverage sequence as Step 9)
5. **FAIL** → `gate-finalize` writes a failure record (`result: fail` + `blocking: true`, `mode: full`, `basis: full`, and the per-check `status` map — `pass`/`fail` for every executed check; full runs re-execute local checks and may carry unchanged relationship judgments — the confirmation state stays visible as BLOCKED and is the failure-recovery baseline), present the findings (Check 2 Step 1 and 5a/5b/5h FAIL findings re-verified per §Extraction re-verification before presentation), and recommend forking the unit (`specflowctl fork --unit <name>`) to reconcile the stable content with the changed dependency or rule. Do not edit stable directly; normal candidate-to-stable writes use promote (the routed remove/update procedures own their narrow exceptions)

The stable confirmation cache is read-only state: it grants no promote eligibility (stable has no gate). Delta re-runs (`revalidate`) apply to stable-only targets with a usable baseline — a pass cache (`result: pass`, STALE recovery) or a failure record (failure-record recovery); a MISSING stable cache needs the full confirmation run — see `framework/verification_scope.md` §Stable-only Targets and §Delta Runs → Layer applicability.

## Execution Rules

- **Subagent permissions:** validate executes one lens-pure session batch per independent read-only sub-agent session — the sub-agent may inspect file content, search text by pattern, locate files by name pattern, query read-only repository history, and run the read-only framework inspection subcommands its checks instruct it to run (`specflowctl deps`, `specflowctl surfaces`, `specflowctl validate candidate`, `specflowctl validate promise-model --layer <candidate|stable>`). It must NOT modify files, run state-changing commands, or delegate to other agents. The main agent sends `specflowctl gate-mission --run {run_id} --keys {keys} --format prompt` output verbatim; its check keys are the session scope (see `framework/verification_scope.md` §Coverage Model). Targeted runs (`:check-{n}` / `:{keyword}`) execute directly in the main agent session instead; they never launch a sub-agent and never create a coverage run or cache write.
- Each check reports **PASS**, **WARNING**, or **FAIL** with a reason.
- On FAIL, the agent must identify **which information sources contradict each other** (e.g., "spec body describes auto-retry logic but no acceptance item covers it") in the FAIL reason, and record the fix in the FAIL reason. When the finding is written out, its `evidence:` block quotes the contradicting sources verbatim and its `fix:` (or `decision:`) field states the repair — the entry line's location reference is a pointer, not the evidence.
- Resolution types:
  - **actionable** — A concrete repair can be made inside the current candidate spec without user judgment.
  - **needs_decision** — Requires user input (unclear intent, missing decision, or external dependency). Stop and ask.

## Output Format

==ATOM_BEGIN:report_skeleton==
## Unified Report Skeleton

### Finding severity ownership

Each finding's `[{severity}]` is assigned by the session that raises the finding. When the final synthesis (`framework/verification_scope.md` §Final synthesis) retains or merges findings, it may raise a retained finding's canonical severity conservatively — never lower it; counts and blocking derive from the canonical severities (see `framework/severity_policy.md`).

### Report skeleton and field definitions

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

- `{gate}@{target}` — the gate and target that produced this report, e.g. `validate@user_auth`, `verify@user_auth`. Gates: `validate`, `verify`. Targets: unit or rule name. The `verify` gate carries two lenses — `alignment` (spec-vs-code) and `quality` (spec-aware code quality) — and its sessions report under one of them. The `alignment` lens runs in item sessions (`item:<unit>:<item>`, Steps 2, 3, 4, and 6, with Step 7 for the mismatches they raise) and in one unit-level scan session per run (`unit:<unit>`, Steps 1 and 5, with Step 7 for its own mismatches).
- `{mode}` — `full` for full runs; `targeted (user requested: {keyword})` for targeted runs; `delta` for delta/repair re-runs (`revalidate@{target}` / `reverify@{unit}`). A **validate** delta adds one change-review session, which has its own contract (`Review result:` + optional `Recheck:` line) and is not rendered with this skeleton; a **verify** delta has no review session and renders with this skeleton.
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
- `Evidence recording:` — reports carry no dependency declarations. `gate-finalize` records the run's whole input surface: one cache entry per input-surface file with its whole-file `hash` and ordered chunk sequence, plus a per-check marker (`check` + lens, and the failure status on failure records) for every executed judgment. Spec objects resolved by name are recorded as logical references so a layer move does not stale the cache (see `framework/validation_cache.md` §Format → Entries). Delta/repair runs record the re-run keys' markers alongside the carried keys' snapshotted markers; targeted runs write no cache.
- `Change review:` — **validate** delta/repair runs only (mode `delta`). The recorded review outcome: the change-set summary (changed files and, per file, the localized change kinds and line spans), the `Review result:` (accept | recheck | escalate-full), and when the reviewer named re-runs, the re-run keys in the run's own structure (e.g. "check-2 (design group): re-run — Description section changed") followed by a line declaring the carried conclusions ("all other conclusions: carried over — recorded content unchanged"). The change set is derived mechanically at plan time (ordered bidirectional chunk diff); the reviewer accepts, names re-run keys, or escalates (see `framework/verification_scope.md` §Delta Runs → The change review (validate)). The mechanical floor re-runs failed and persisted-invalidated checks (repair), current checks the baseline never declared, a moved judging standard, and explicit `--rerun` overrides regardless of the review. A failure record whose per-check status map is absent, invalid, or inconsistent with its judgment baseline, or a failed or invalidated key that cannot map to the current surface, is covered by a full-scope plan — nothing is carried over. The review result and the planned scope are reported by `gate-plan` before execution begins (the user must see what will be re-run and what will be carried over) and again in the final report. A **verify** delta has no review record: its re-run scope is derived from the conclusion records, and the final report states the re-run keys and the carried conclusions instead.
- `Next step:` — the concrete command to run next with its reason; `None` when nothing further is needed. A finding's fix lifecycle has three states with fixed wording: `finding_open` → "Resolve the findings, then re-run the target-appropriate re-check command (`validate@{target}:check-{n}`; unit targets also `verify@{target}:{keyword}`) to confirm"; `fixed_pending_recheck` → "Fixes applied; re-run the target-appropriate re-check command to confirm." — only after the approved fix was actually written; `verified` → "Re-check passed." — only after a re-check confirmed the fix. A gate report is always produced before any fix is applied (nothing is implemented before the user approves the findings), so an actionable finding's report-time `Next step` is always the `finding_open` wording. Other guidance: both gates green → "if the design is finalized, run `promote@{target}`"; needs_decision → "awaiting your decision on {item}"; nothing further → `None`.

**Targeted runs:** end the report with the gate's targeted note ("This was a targeted check — no complete cache was written. Run `{gate}@{target}` for a complete ...") after the `Next step` line. If the result contains P0/P1, run `gate-invalidate` before reporting completion.

### Completion — Persist Gate Cache

A quality-gate run is complete only when its result is persisted and visible to `fresh`/`promote`. A report alone does not satisfy the gate.

- **Full (`validate@{target}` / `verify@{unit}`, `mode: full`, `basis: full`)** — complete only when the coverage run finished and its cache was written: (1) `specflowctl gate-plan --gate {validate|verify} (--unit {name} | --rule {id}) --target {candidate|stable} [--relationships NAMES|none] [--inputs-file PATH]` fixed the immutable input snapshot, local coverage set, and relationship scope (`--relationships` is required for unit runs; select names or `none`) **before any executor read input** (before mission assembly / sub-agent launch) — the input manifest adds evidence that every session may read but never creates a coverage key, and it must not be a repository-content file (keep it under the ignored `meta/plan_inputs/` or outside the repository); (2) the tooling's computed reviewer units for the uncovered coverage were joined by the coordinator into sessions — one per report kind and lens (one per reviewed file for verify quality), and a session's key set is split only when a session declares it exceeds that session's capacity — reusing accepted public tasks and waiting for assigned tasks and, for each session, generated a mission with `specflowctl gate-mission --run <run_id> --keys <k1,k2,...> --format prompt`, sent it verbatim to one independent reviewer, and recorded its report with `specflowctl gate-submit --run <run_id> --session <session_id> --keys <k1,k2,...> --report PATH` until `gate-status` reports full coverage; (3) when relationships are assigned or the primary pass produced findings, one independent final synthesis was generated with `specflowctl gate-mission --run <run_id> --final --format prompt` and recorded with `specflowctl gate-submit --run <run_id> --session cross --keys cross --report PATH`; a run with neither assigned relationships nor findings skips this step; (4) the coordinator ran `specflowctl gate-finalize --run <run_id>` and the cache `docs/specs/meta/validation/{unit|rule}/{name}/validate_result.md` | `verify_result.md` was written (the merged verify cache carries its `alignment` and `quality` sections); (5) `fresh@{target}` (`fresh --unit {name}` / `fresh --rule {id}`) shows the expected gate state (`FRESH` for `result: pass` / `blocking: false`, `BLOCKED` for `result: fail` / `blocking: true`). A candidate **full-run** FAIL writes a failure record (`result: fail` / `blocking: true`; validate/verify records declare `pass`/`fail` for every executed judgment) — the failure-recovery baseline. A finalize that finds any input changed since `gate-plan` is rejected and writes no cache. See `framework/validation_cache.md` §Write Rules and §Failure handling by gate role.
- **Delta (`revalidate@{target}` / `reverify@{unit}`, `mode: full`, `basis: delta` or `basis: repair`)** — the same coverage-run sequence. A **validate** delta adds one review session: `specflowctl gate-plan --mode delta` computes the change set, schedules the review, and mechanically forces failed and persisted-invalidated checks (repair), current checks the baseline never declared, and explicit `--rerun` overrides; the review report (`Review result:` + optional `Recheck:`) may add re-run keys; `gate-finalize` merges the reviewed scope's fresh evidence with the carried baseline evidence and records the review. A **verify** delta has no review session: `specflowctl gate-plan --mode delta` derives the re-run set from each conclusion's own record pins and from the evidence its key currently requires (a new file in a declared surface), adds current keys the baseline never declared and explicit `--rerun` overrides (repair also forces failed judgments, persisted invalidations, and failed relationships), and carries the rest; the whole derived set executes under the standard checklist, and `gate-finalize` merges the re-run evidence with the carried baseline evidence without a review record. See `framework/verification_scope.md` §Delta Runs.
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
  5h. Promise block semantics: PASS | FAIL — reason
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

**Per-item merge rule:** Findings from sub-check 5e that reference the same acceptance item (same location) and whose fix is a rewrite of that item are merged into **one finding**: the merged entry's issue line summarizes the item defect, and its `evidence:` block quotes each violated rule and the offending text. The merged entry's severity is the highest among its violated rules (P0 > P1); a P0 grade still requires the §9 boundary check per the Severity handling section. Findings from 5a (spec body regions), 5b (each contradiction is an independent fix edit), and 5h (promise block findings) are not merged — their locations and fixes differ.

When findings mix resolution types (within one check or across checks), the report presents each finding with its own resolution — a `needs_decision` finding stops the flow and requires user input per Execution Rules.

**Evidence discipline:** every check line's reason must reference the spec content the verdict covers (section title, item id, appendix name). A PASS with no reference means the check did not run — report FAIL with "no evidence basis". The check-line reference is the verdict trace; a finding separately carries the quoted content in its `evidence:` block.

---

## Check 1 — Structural integrity

**Purpose:** Verify the file is parseable and all required fields exist, as a prerequisite for all subsequent checks.

**Mechanical pre-pass (candidate targets only, before gate-plan):** for a candidate target, run `specflowctl validate candidate --unit {name}` first. A stable-only target runs no mechanical pre-pass — the tool checks the candidate layer only (see §Target-layer applicability) — so the structural session judges the stable content directly. The tool deterministically enforces: frontmatter fields; acceptance item schema (required fields; `testable` items need a Gherkin-style Given/When/Then description, and `.feature` syntax is rejected) and `implementation_surface` resolution (a value inside the spec tree is rejected — a spec document is a reference object, not a code surface); `affects.files` and `affects.evidence_files` path resolution and existence (`affects.files` rejects a spec document on the same rule); `rule_refs`/appendix existence; candidate-layer spec paths in the body and every appendix; section/region locatability (at least one `##` heading, unique headings, malformed headings, frontmatter region purity, unique non-empty item ids); reserved content headings (changelog/revision section headings — FAIL; `framework/spec_writing_guide.md` §15.1); `verification_method` shape (a step-by-step recipe — FAIL); prose path hygiene (WARNING); environment-specific content — developer-machine absolute paths, local addresses, and credential patterns (narrative hits FAIL; unmarked fenced hits WARNING); process traces (WARNING); and test-double names (WARNING); and the promise model (shape, schema, coverage, and graph — a unit without `## Promises` fails). A mechanical FAIL stops the run before any session launches — fix and re-run the tool. Structural warnings (prose path, environment residue) are passed to the structural session; content-boundary warnings (process traces, test doubles) are passed to the Check 2 Step 5 session for confirmation.

**Agent-judged residue (the structural session executes these):**

1. Read the main spec and all appendix files from the resolved layer (see Prerequisite)
2. **Stable-layer spec paths in prose (FAIL):** the tool flags candidate-layer paths everywhere but cannot distinguish structured fields from prose for stable-layer paths — a stable-layer spec path (`docs/specs/units/stable/...`, `docs/specs/rules/stable/...`, or a relative `stable/...*.md` form) in narrative prose mispoints after the referenced unit promotes. Path-valued structured fields that may reference a spec document (`affects.evidence_files`, `affects.appendices`) may hold stable-layer paths; `implementation_surface` and `affects.files` declare implementation code, so a spec-document value on either is rejected rather than layer-checked (see the mechanical pre-pass above and `framework/spec_writing_guide.md` §7, §12). If found → FAIL with quoted path, section, and line reference. Reference appendix files and other specs by concept name or file name instead
3. **Prose-path hygiene confirmation (WARNING):** review the tool's WARNING hits plus any source-path pattern the scanner cannot express — relocate to `implementation_surface`, `affects.files`, or `affects.evidence_files`, or convert to a concept name reference. Exclusions mirror the tool's: structured fields, `framework/` governance paths, `docs/specs/meta/` cache paths, marked fenced examples
4. **Region structure semantics check (FAIL):** the section regions must match the content's semantic structure — the acceptance item set's boundary and the item-region pins of item judgments are only as correct as the split. Reading the spec itself, verify:
    - **The acceptance_item_set region covers every real item** — the region runs from the exact `acceptance_item_set:` marker line to the next `##` heading outside a code fence (the enclosing section's end), or through the last real line of the file. `###` and deeper headings never terminate the set: an item separated from the previous one by a `###` subheading is still part of the set and is verified. An item placed after the enclosing section's `##` heading sits outside the set and cannot be validated as an item — catch that here. Distinguish real items from fenced example blocks — a fenced `- id:` example is content, not an item (the mechanical pre-pass enforces the exact-marker and heading-format preconditions; the semantic coverage judgment is this step's)
    - **Fenced code blocks are content** — `##`-like lines inside ``` / ~~~ fences must not split regions; when a fence would visually span a section boundary, the spec needs restructuring (a fence cannot cross `##` headings — close the fence before the next heading)
    - **Headings are unique and well-formed** — every `##` heading resolves uniquely (a duplicated heading cannot locate an item set or an item region; the mechanical pre-pass enforces the formal preconditions, the semantic judgment is this step's)
    - **The acceptance item set is the last content (`framework/spec_writing_guide.md` §13.9)** — nothing follows the item set: no heading of any level (`#` through `######`, not only `##`), no `---` divider or bold pseudo-title, and no narrative paragraph. This is a semantic judgment (the trailing forms cannot be enumerated mechanically), so this step owns it. Whatever appears after the item set — a design, budget, or alignment section, or trailing prose — is misplaced (actionable: move it before the acceptance item set)
    - **The acceptance section contains only the item set** — `###` subheadings are not allowed inside `## Acceptance Items` (Check 15 fails them); narrative belongs in `## Design Narrative`. Any narrative or set-level content inside the item set is attributed to the last item's region (actionable: move it out)
    - Fix direction: restructure the spec so the split is semantically clean (cohesion per `framework/spec_writing_guide.md` §13)
5. **Environment agnosticism judgment (FAIL):** the mechanical scan flags the unambiguous patterns; the session judges the residue — whether a narrative value is a hard requirement (FAIL) or an explicitly marked example placeholder (pass), and credential-like content the pattern scan cannot recognize. Must NOT contain fixed local machine IP addresses or ports as hard requirements, or live or environment-specific credentials, tokens, or private secrets. If found → FAIL with quoted text, section name, and line reference (actionable: replace with project-scoped relative paths, abstract placeholders, or configuration-driven parameters)

**PASS:** For a candidate target, the mechanical pre-pass passed and the residue judgments found no defect; for a stable-only target, the residue judgments found no defect (no mechanical pre-pass runs)

**WARNING:** Prose-path hygiene hits and unmarked fenced environment content — surfaced by the tool, confirmed or cleared by the session

**FAIL:** Any mechanical pre-pass failure (candidate targets), a stable-layer spec path in narrative prose, environment-specific hardcoding in prose or structured fields, or a section-order violation (`framework/spec_writing_guide.md` §13.9) (actionable)

**Check method:** For a candidate target, deterministic tool pre-pass (`specflowctl validate candidate`) + agent judgment on the semantic residue; for a stable-only target, agent judgment on the structural residue alone (no mechanical pre-pass)

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
- A finding without this artifact is not presented — the independent final synthesis re-verifies the artifact before classification and marks unfaithful claims suppressed (see §Extraction re-verification; e.g. a behavior that does serve a stated goal, or a proportionality claim that rests on preference rather than an establishable smaller surface, drops the finding).

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

- **Input discipline:** the spec text is the ONLY input (main spec + appendices). Do not fill spec gaps with implementation knowledge from this session — if the spec omits a decision, the gap is real and must be reported. Reading the implementation defeats this check's purpose: a spec that only makes sense with code knowledge forces the downstream executor to choose, which is exactly what the baseline forbids.

**Decision closure (the "must close" list):** Verify that the spec closes every implementation-affecting decision: which object owns a responsibility, which entry point starts the behavior, where state or durable truth lives, how ordered steps connect, how boundary failures are reported, what the result shape means, how acceptance proves the stated responsibility.

- For each of the seven decisions: can the downstream executor determine the answer from the spec alone, without making a choice?
  - For evidence-driven acceptance items (Step 2 waiver), the closure source is the evidence appendix: verify it records the item's behavior domain as directly readable behavioral truth (per `framework/spec_writing_guide.md` §3 `evidence_appendix_ref`), not only background, motivation, or patch notes.
  - The "how acceptance proves the stated responsibility" decision is covered by Check 5's acceptance quality review — here, only confirm the acceptance items exist and can prove the stated responsibility.
- If a decision is intentionally not made, the spec must state that boundary and explain why (per `framework/spec_writing_guide.md` §9, the "If a decision is intentionally not made" rule). An open decision without a stated boundary is a FAIL.
- Granularity: verify closure, not exhaustiveness — the spec must not be inflated into an implementation manual. Coverage obligations are limited to formal behavior domains (see Check 5a Step 2 extraction premise); narrative elaboration in the body does not add coverage obligations.

**FAIL:** a Step 1 goal-means flag (default severity P1; extraction artifact required), a Step 2 rationale gap, a Step 3 critical flaw, a Step 5 content-boundary violation (P1, `framework/spec_writing_guide.md` §15; needs_decision when the §15.2 boundary is undeclared); or any of the seven decisions is left open AND not explicitly bounded with a reason (actionable: record the decision / declare the boundary; needs_decision when recording it requires user input — Execution Rules "missing decision")

**Step 5 — Spec content boundary (self-content)**

This step owns the content boundary for the unit's **own** content (body narrative + appendices), per `framework/spec_writing_guide.md` §15, except the §15.2 external-visibility judgment on an acceptance item's own subject, which is owned by Check 5a step 9. Cross-unit shadow specifications — content this unit states about a **collaborating unit's** internals — remain owned by Check 7 step 8; the three never report the same defect. The mechanical pre-pass surfaces process-trace and test-double WARNINGs (guide §15.1, §15.3); this step confirms each hit and judges the semantic residue (internal implementation). The input-discipline rule of Step 4 applies here too: the spec text is the only input.

1. **Current truth only (§15.1):** the unit's own content carries change history or process traces → FAIL (actionable: delete the history — version control owns it; keep the replacement's rationale, which is current design expression).
2. **Externally observable only (§15.2):** apply `framework/spec_writing_guide.md` §15.2's default dispositions to the unit's own content. A statement on the internal side (disposition 2) → FAIL (actionable: delete the internal narration and keep the externally observable behavior it served, expressed as behavior). Content whose §15.2 boundary is undeclared (disposition 3) → FAIL (needs_decision per §15.2).
3. **Not stated elsewhere (§15.3):** the same statement is carried more than once, design narration duplicated into the acceptance item set → FAIL (actionable: keep the condition in the item; the mechanism narration lives once, in the design narrative).
4. **Wall-clock sleeps and brittle temporal waits (§14.2 Anti-Pattern D) in this unit's own prose** → FAIL (actionable: restate as a causal state condition with an explicit timeout boundary).

Each finding quotes the offending passage and names the clause violated. §15 is the single authoritative definition of each clause; this step cites it and does not restate it.

**Step 6 — Verdict**
- PASS: goal-means aligned, per-item rationale documented (evidence-driven items waived per Step 2), all seven §9 decisions closed or explicitly bounded, no critical flaws found, and no §15 content-boundary violation (Step 5)
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

**Purpose:** The spec body and acceptance items must cover each other bidirectionally — every designed behavior has an item (5a forward coverage), and every design-driven item maps back to a designed behavior (5a orphan detection; evidence-driven and replacement/cleanup items excepted). Design narrative, promise contract, and item content must match semantically and contain no internal contradictions (5b), and every acceptance item must be falsifiable, actionable, and carry contract substance beyond its description (5e).

**Execution steps:**

### Sub-check 5a — Coverage & item-set correspondence

**Purpose:** Every behavior domain in the spec body and appendices must have at least one corresponding acceptance item, every design-driven acceptance item must correspond to a designed behavior (orphan detection, step 7), and the item's surface fields must be consistent with the behavior type. Granularity baseline: behavior domains as defined in `framework/spec_writing_guide.md` §Acceptance Item Granularity — one item = one behavior domain with its full scenario set (happy path + error paths + boundary cases). Enhanced from the original forward coverage check to a bidirectional check.

**Execution steps:**

1. **Coverage input source:** A behavior is covered when any acceptance item describes it in its `description` (Given/When/Then scenarios) OR constrains it in its `pass_condition`. The coverage judgment input is the union of `description` and `pass_condition` — a behavior constraint that already appears in some item's `pass_condition` counts as covered, consistent with sub-check 5e (whose information-increment rule requires `pass_condition` to carry constraints beyond `description`).
2. **Extraction premise (shared with sub-checks 5b and 5h):** Behavior-domain extraction targets only formal behavior declared in the spec body and appendices — a behavior subject (endpoint, function, state machine, or flow entry point) together with its behavior semantics. The subject must be externally observable per `framework/spec_writing_guide.md` §15.2: internal implementation detail is design expression, not a behavior domain source, and creates no coverage obligation — the same boundary applies to appendix contract content in step 6 and to the design narrative in sub-check 5h. Non-constraining narrative (design discussion, illustrative examples, motivation, variant elaboration) is NOT a behavior domain source and does not create coverage obligations. Extract all behavior domains at the granularity defined in `framework/spec_writing_guide.md` §Acceptance Item Granularity: group behavior variants around one behavior subject into one domain (error paths, boundary cases, and state transitions of the same subject are scenarios of that domain, not separate domains); do not split scenarios of the same domain into separate coverage requirements.
3. For each behavior domain, verify at least one acceptance item covers it (using the union input from step 1)
4. For each covered domain, verify the item's `implementation_surface` and `verification_surface` are consistent with the behavior's nature (e.g., REST API behavior should have surface `api`, not `db`)
5. If a behavior domain has no acceptance item → flag (possible untested behavior)
6. **Appendix contract boundary:** Extract all behavior domains, API contracts, data type definitions, and state machine transitions from appendix files — apply the external-visibility boundary of `framework/spec_writing_guide.md` §15.2 first: internal implementation detail is design expression, not contract content, and creates no coverage obligation. Normative contract content must be carried by a promise in the main spec's `## Promises` section. If an appendix declares contract content that no promise carries → **FAIL (actionable)** — move it into a promise; an appendix is explanatory support truth. If the appendix contradicts a promise or an acceptance item (e.g., appendix says "timeout: 30s", promise says 5s) → FAIL (actionable). If an appendix describes behavior that no acceptance item covers, judge it as a behavior domain under step 3; do not require a second carrier in the appendix.
7. **Orphan item detection (reverse check, FAIL):** For each design-driven acceptance item — an item whose `affects.appendices` does not reference the evidence appendix (Check 4 step 1) — verify the item's behavior subject (endpoint, function, state machine, or flow entry point, per the extraction premise in step 2) appears as a designed behavior somewhere in the candidate spec union: the main spec body or a non-evidence appendix. The subject may be designed as part of a larger flow; the test is subject presence, not narrative repetition of every contract element — the verification obligation runs one way, and the normative contract elements are carried by the promise, not the item (`framework/spec_writing_guide.md` §4). Exclusions: evidence-driven items are out of scope (their correspondence partner is the evidence appendix, enforced by Check 4); replacement/cleanup items are out of scope — `verification_type: inspectable` items whose `evidence_requirements` include `old_code_deleted` and `no_remaining_refs` are round-transitional verification requirements, not behavior declarations (the Check 4 step 1 replacement signal). If the subject appears in no designed behavior → **FAIL (P1, actionable):** retire the item together with its surviving narrative and appendix content (`framework/spec_writing_guide.md` §9 Cleanup obligation), or restore the behavior's design if it was dropped by mistake.
8. **Extraction evidence (required for every uncovered-domain and orphan-item FAIL):** Each uncovered-domain finding (steps 3, 5, and step 6's uncovered-content case) and each orphan-item finding (step 7) must carry an extraction artifact that makes the claim falsifiable:
   - **Source quote:** for an uncovered domain, the section heading and quoted text in the spec body or appendix that declares the behavior domain; for an orphan item, the item id and the quoted subject terms from its `description` / `pass_condition`
   - **Granularity judgment:** for an uncovered domain, why the quoted text is formal behavior (per the extraction premise in step 2, including the §15.2 external-visibility boundary applied to both body behavior domains and appendix contract content) rather than non-constraining narrative or internal design detail, and why its behavior variants form one domain (per the four granularity conditions in `framework/spec_writing_guide.md` §Acceptance Item Granularity) rather than scenarios of an already-covered domain; for an orphan item, why the quoted terms name a formal behavior subject of this unit (endpoint, function, state machine, or flow entry point) rather than a scenario of a designed domain or a dependency's behavior
   - **Absence claim:** for an uncovered domain, the covered surface checked (the union of every item's `description` and `pass_condition`, per step 1) and how the absence of any covering item was verified; for an orphan item, the surfaces checked (the main spec body and every non-evidence appendix) and how the absence of a designed behavior for that subject was verified
   A step-6 contradiction finding (appendix content contradicting an acceptance item) carries the two-sided quoted evidence step 6 itself requires — it is a conflict claim, not an uncovered-domain claim, and is not subject to this template.
   A finding without this artifact is not presented — the independent final synthesis re-verifies the artifact before classification and marks unfaithful claims suppressed (a subject actually mentioned in an item, or variants split out of a covered domain, or a designed behavior whose subject matches an allegedly orphaned item — see §Extraction re-verification).

9. **Item subject observability (FAIL / needs_decision):** apply the external-visibility judgment of `framework/spec_writing_guide.md` §15.2, on its declaration-surface footing, to each acceptance item's own subject — the behavior subject named in its `id`, `description`, and `pass_condition`. An item that declares `non_promise: true` while its subject is an externally observable contract behavior (an endpoint or callable surface, an event, an exchanged data schema, a state machine, or a standing property) is not a non-contract item: **FAIL (actionable):** name the promise that carries the behavior, or remove the contract content from the item. An item whose subject is an internal implementation fact — an internal data structure and its operations, an internal buffer size or capacity, an internal ordering or block layout, an internal retry/backoff value, an internal rendering step, an internal protocol mapping — is not a behavior domain: it states how the unit is built, not what a declared consumer reads or is bounded by → **FAIL (actionable):** retire the item together with the narrative it mirrors per §9 Cleanup obligation, or rewrite it as the externally observable condition that internal decision serves, with the consumer-visible effect carrying the contract elements of §7. A subject that is an attribute of the unit's own design record — its main spec's or appendix's sections, headings, or field values, or any other property of the Spec document itself (for example "the unit's document contains six sections") — is likewise not a behavior domain → **FAIL (actionable):** retire the item; the document's structure is a framework format constraint enforced mechanically on the file (Check 15, `framework/spec_writing_guide.md` §13), not an acceptance item, and this boundary covers the record rather than the product — a document the unit produces for its consumers is the unit's output, whose observable properties are stated as behavior. The finding's evidence is the item's own subject wording. A subject the spec marks internal (§15.2 disposition 2) is a direct hit: the finding's evidence is the subject's own wording, with no negative-existence proof required. A subject whose §15.2 boundary is undeclared (disposition 3) → **FAIL (needs_decision per §15.2):** the item is not passed by default. Each finding quotes the item id and the subject terms it judges, and carries the positive artifact the final synthesis re-verifies: the declared boundaries checked, and which declared consumer, if any, reads, is bounded by, or is affected by the subject. This step owns the §15.2 judgment for acceptance item subjects; Check 2 Step 5 owns the body narrative and appendix content and does not re-report an item subject.

**PASS:** All behavior domains (main spec + appendices) have corresponding items with appropriate surface fields; every design-driven item corresponds to a designed behavior; every item's subject is externally observable

**FAIL:** Uncovered behavior domain or surface type mismatch (actionable); orphan design-driven item with no corresponding designed behavior (step 7, actionable); item whose subject is not externally observable (step 9, actionable); item subject whose §15.2 boundary is undeclared (step 9, needs_decision); appendix contract content without a promise carrier (actionable); appendix-promise contradiction (actionable); narrative contract statement without a promise carrier or promise without narrative support (5h, actionable); promise-item contradiction (5b, actionable); item violating the item-substance rules (5e, actionable)

**Check method:** Spec body + appendices × acceptance item set bidirectional cross-reference (body → items for coverage; items → body for orphan detection)

---

### Sub-check 5b — Semantic consistency

**Purpose:** Detect semantic contradictions between the design narrative, a promise's normative contract, and the acceptance items that prove it, and between items describing the same behavior. This catches narrative or promise edits that invalidate item content — whether from recent changes or historical drift. A promise-item contradiction is a design defect: the item cannot prove the promise it names.

**Execution steps:**

1. **Design narrative × item:** For each behavior declared in `## Design Narrative`, identify its corresponding acceptance items. Read the narrative description and the item's `description`/`pass_condition`, then apply natural language reasoning to identify semantic contradictions:

| Contradiction type | Narrative says | Item says |
|---|---|---|
| Value conflict | "timeout: 30s" | "respond within 5s" |
| Behavior conflict | "login accepts email+password" | "return error when password provided" |
| Scope conflict | "supports OAuth and API Key" | "only validates API Key" |
| Direction conflict | "increment counter" | "decrement counter" |

2. **Promise contract × item:** For each item with a `promise` value, and for every item whose subject matches a promise's behavior subject even when it declares `non_promise: true`, read the referenced or matching promise's `contract`. Compare the item's `description` and `pass_condition` against every normative field the promise states:
   - **Value conflict:** status code, numeric limit, timeout, TTL, rate, field/type/enum name, error code, protocol name/format, event name/payload/delivery/ordering, interface or function signature, invariant, or state/transition differs.
   - **Scope conflict:** the item declares a different behavior subject (a different endpoint, event, schema, callable surface, or state machine) than the promise, or says a case the promise requires is not supported.
   - **Behavior conflict:** the item declares behavior the promise prohibits, or the promise declares behavior the item denies.
   A direct contradiction → FAIL (actionable: correct the item or the promise so both state the same contract). An item that tests one scenario of the promise is not a contradiction; a missing scenario is coverage, owned by 5a, not this check.
3. **Items × items (internal consistency):** group items sharing a `verification_surface`, and items sharing an `affects.files` entry; compare their `pass_condition` texts for contradictions — value conflicts (item A says "returns 201", item B says "200" for the same API), behavior conflicts (item A says "write requires auth", item B says "write is public"), numeric contradictions (one says <100ms latency, another <5s for the same operation), and logical contradictions (one says "enabled by default", another says "opt-in only"). Report each with quoted evidence from both items.
4. For each contradiction, report with **exact quotes** from both sources and a reasoning statement. For a promise × item contradiction, quote the promise field path (for example `contract.errors.401`) and the item field text.
5. **Extraction evidence (required for every promise × item FAIL):** quote the promise field and the item text, name the normative field, and state why the two cannot both hold. The independent final synthesis re-verifies the pair; a value the item narrows to a legal subset, a promise range that contains the item's stricter bound, or an item that adds a stricter case without contradicting the promise is not a finding.

**PASS:** No contradictions among the design narrative, promise contracts, and acceptance items, and among items describing the same behavior

**FAIL:** One or more contradictions found, with quoted evidence (actionable)

**Check method:** Design narrative × item, promise contract × the items that name it, and item × item (by verification_surface and affects.files) — semantic cross-reference with quoted evidence

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
5. **Boundary:** the specific values S1/S2 require must be genuine Contract Anchors owned by this unit or public contracts exported by dependencies (`framework/spec_writing_guide.md` §14, §15.2); internal implementation detail of this unit does not satisfy contract substance (judged by Check 5a step 9), and a collaborating unit's unexported fields do not either (judged by Check 7 step 8).
6. **`verification_method` substance (`framework/spec_writing_guide.md` §15.3):** the field states the method in one sentence. A step-by-step test plan ("verify (1) … (2) …"), an expected call sequence against a stub, or a test-double name (`stub X`, `mock Y`, `fake Z`) → FAIL (actionable: keep one sentence naming the method and surface; the plan lives in the test code).

**PASS:** All items are falsifiable, actionable, and carry contract substance beyond their descriptions

**FAIL:** One or more items violate any rule above (quote the violated rule and the offending text)

**Check method:** Per-item semantic assessment with quoted evidence; one item's violations are reported as one merged finding (see Per-item merge rule).

### Sub-check 5h — Promise block semantics

**Purpose:** The promise block's semantics. Contract statements in the design narrative must be carried by a promise, every promise must be grounded in the design narrative, each promise's `contract` must carry exactly one behavior subject, every promise's contract must stay contract-only, and every contract block must state the observation mode it claims. The promise block is the formal behavior carrier (`framework/spec_writing_guide.md` §4): a contract that lives only in prose is invisible to every dependent unit, a promise with no narrative basis is an unsupported contract, a bundled promise recreates whole-file invalidation one level down, an implementation/verification detail inside a promise makes consumers depend on internals, and a structural fact hidden as a free-text invariant escapes the block vocabulary that makes it comparable. The narrative obligation is scoped to the external-visibility boundary of `framework/spec_writing_guide.md` §15.2: only statements declaring the unit's externally-observable behavior are taxed — internal implementation detail is design expression and creates no promise obligation. Item↔promise consistency is owned by sub-check 5b; this sub-check does not re-report it.

**Execution steps:**

1. Extract **contract statements** from the `## Design Narrative` section (including its `###` subsections):
   - Apply the external-visibility boundary of `framework/spec_writing_guide.md` §15.2 first: a statement declaring the unit's externally-observable behavior — what a declared consumer must read to use this unit safely (API surface, exchanged data formats, error codes, protocol semantics, timing/consistency guarantees) — is a contract statement; internal implementation detail is not, even when it states concrete values
   - Then classify each statement by type:
     - Numeric constraints (timeout, rate, limit, TTL values)
     - HTTP status codes and error codes
     - Field / type / enum names and data formats
     - Protocol names and formats
     - Timing / consistency assumptions (sync vs async, ordering guarantees, consistency expectations)
   - Non-constraining narrative (design discussion, illustrative examples, motivation) is NOT a contract statement — same judgment principle as sub-check 5a's extraction premise (see 5a Step 2)
2. For each promise, verify its `contract` carries **contract content only**: no code file path, code region, implementation mechanism, verification method, test evidence, or acceptance-item field. A promise that carries implementation or verification detail → FAIL (actionable: move it to the acceptance item or design narrative). This is the content half of the mechanical field-shape boundary; the mechanical schema closes the field set, this step closes free-text content.
3. For each contract statement, verify a promise carries it at a **comparable granularity**: the promise's `contract` states the same value, code, field name, format, or assumption. A statement carried only by an acceptance item or appendix does not satisfy this check.
4. For each promise, verify it is grounded in the design narrative: the design rationale, responsibility, or behavior that justifies the promise is stated in `## Design Narrative`. A promise with no narrative basis is an unsupported contract.
5. For each promise, verify its `contract` carries **one behavior subject** (`framework/core/promise_model.md` §Granularity): one endpoint or callable surface, one event, one exchanged data schema, one state machine, or one standing property. A promise whose `contract` bundles several subjects — an `operations` block describing multiple unrelated interfaces, a `data` block unioning several exchanged types, or a `state_machine` block mixing two lifecycles — fails this step even when every field is individually well-formed.
6. For each promise, verify each contract block states the fact its name claims: `operations` states callable surfaces, `data` states record shapes, `occurrences` states provider-initiated events, `state_machine` states a mode/transition contract, and `invariants` states a standing property with no structural shape. A structural fact placed in `invariants` when a structural block can carry it → FAIL (actionable: move it to the matching block).
7. A contract statement with no promise carrier → FAIL (actionable: move the contract into a promise). A promise with no narrative support → FAIL (actionable: add the design rationale to the narrative or retire the promise). A promise carrying implementation/verification detail → FAIL (actionable: move the detail to the item or narrative). A bundled promise → FAIL (actionable: split it per behavior subject and migrate the consumers' `depends_on` and this unit's `promise` item references). A structural fact placed in `invariants` → FAIL (actionable: move it to the matching block).
8. **Extraction evidence (required for every FAIL finding):** Each uncarried-contract finding must carry an extraction artifact that makes the claim falsifiable:
   - **Source quote:** the section heading and quoted sentence in the design narrative that states the contract, or the promise id and quoted contract for an unsupported promise
   - **External-visibility judgment:** why the statement declares externally-observable behavior (per the §15.2 external-visibility boundary) rather than internal design detail
   - **Absence claim:** the promise surface checked (every promise's `contract`) and how the absence of a carrier at comparable granularity was verified; for an unsupported promise, the narrative surfaces checked and how the absence of design rationale was verified
   A contract-content finding quotes the offending promise field; a granularity finding quotes the bundled `contract` and names the behavior subjects inside it; a block-choice finding quotes the misplaced fact and names the block that can carry it. A finding without this artifact is not presented — the independent final synthesis re-verifies the artifact before classification and marks unfaithful claims suppressed (a value actually carried, or a statement reclassified as internal detail — see §Extraction re-verification).

**PASS:** Every narrative contract statement is carried by a promise, every promise is grounded in the narrative, every promise carries one behavior subject, every promise's contract text is contract-only, and every contract block states the observation mode it claims

**FAIL:** One or more contract statements lack a promise carrier, one or more promises lack narrative support, one or more promises carry implementation/verification detail, one or more promises bundle several behavior subjects, or one or more structural facts are placed in `invariants` (actionable)

**Check method:** `## Design Narrative` × `## Promises` — contract-level cross-reference, per-promise content/subject/block judgment, quoted evidence per statement or promise

---

## Check 6 — Affects-source validity

**Purpose:** Each acceptance item's `affects` declarations must be consistent with the spec's formal references. Evidence appendix content must be structurally sound and semantically meaningful.

**Execution steps:**

1. For each acceptance item, verify:

```
affects.rules:
  - Each rule must be either a global rule or listed in frontmatter rule_refs
  - Referencing a non-existent or undeclared rule → FAIL

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

3. **Appendix file path references:** For each appendix, scan its content for code file path references (strings containing `/` and a source-code file extension). For each path found, verify it points to an existing file in the project. If any path does not exist → FAIL (actionable: update or remove the invalid path reference)

**PASS:** All affects declarations are valid, evidence appendix is semantically consistent, and appendix file path references exist

**FAIL:** Reference inconsistency, appendix content contradicts declaration, or appendix references non-existent file paths (actionable)

**Boundary — declared-vs-actual surface is verify's judgment:** whether a declared `implementation_surface` / `affects.files` file actually participates in the item's behavior requires reading code, so it is owned by `framework/unit_verify_checklist.md` Step 3 (declared vs actual surface), which covers both over-declared and undeclared scope. Validate judges the declaration itself — resolution, ownership, path existence, mutual consistency — and reads no code; validate's read surface carries no implementation files.

**Check method:** affects.* × frontmatter refs cross-reference + appendix content semantic assessment

---

## Check 7 — Cross-unit consistency

**Purpose:** The target spec must not contradict related units. A candidate validate checks the current layer (candidate providers preferred, stable fallback); a stable-only validate checks the published stable layer. Unacknowledged contract changes must be flagged.

**Execution steps:**

1. Build the dependency unit set from the provider units named by this spec's promise `depends_on` edges. Resolve each provider in the target layer's promise scope: a candidate validate resolves providers candidate-first with stable fallback; a stable-only validate resolves only published stable providers.
2. For each dependency unit, read its **promises** — the `## Promises` block in the main spec resolved in that scope (candidate-first for a candidate validate; published stable for a stable-only validate). The provider's acceptance items, appendices, body prose, and code are not its contract: a consumer reads the provider's promises. Promise completeness is guaranteed by the dependency unit's own validate (Check 15 enforces the promise model; Check 5a/5b/5h enforce coverage, item-proof consistency, and the narrative/appendix boundaries), so reading the provider's promises reads its complete formal contract.
3. Candidate validate: the provider's candidate spec takes priority — check its promises for conflicting statements about shared protocols, data formats, or behavior; when the candidate names specific contract symbols (protocol names, field/type names, error codes), locate and verify those exact promise regions first.
4. Candidate validate only: when the provider has a candidate, the candidate validate plan also carries the provider's published stable main spec as read-only comparison evidence (a physical stable ref, not a dependency pin). Read it when the comparison requires the published contract; keep the provider's candidate promises as its current contract. If the provider has no stable main spec, there is no published contract to compare — report it as an unpublished provider dependency (an external blocker named in the report), not as a missing read ref; promote rejects the consumer until the provider is promoted. For a stable-only validate, the published stable provider promises are already the step-2 source.
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
   For a stable-only validate, read "this candidate" as the stable target and
   compare against published stable providers; a candidate provider draft is not
   in scope.
6. **Provider promise cross-unit check:** Compare the candidate's contract statements (its promises and the narrative that explains them) against the dependency unit's promises. If a provider promise and this unit's contract conflict on a shared protocol, data format, or behavior → FAIL (actionable: resolve the cross-unit inconsistency). A provider acceptance item or appendix that conflicts is not a provider contract; report it only if the provider spec presents it as normative contract content, which Check 5a/5h should already have moved into a promise.
7. **Shared agreements and dependency declarations (folded from Check 9):** shared behavior, data meaning, and boundary agreements must have one authoritative source — shared constraints live in rules consumed through `rule_refs`, not restated independently in several specs. Dependencies are declared on the consumer promise's `depends_on`; there is no other dependency channel. Never infer a dependency from a shared file alone.
8. **Cross-Unit Private Implementation Leakage (Anti-Shadowing Rule, FAIL):** This step is the single owner of the **cross-unit** application of the Truth Ownership rules (`framework/spec_writing_guide.md` §14.1–§14.2 Anti-Pattern A) and of §15.2 as applied to content this unit states about **collaborating units'** internals. The unit's own content is owned by Check 2 Step 5 (body narrative and appendices), Check 5a step 9 (acceptance item subjects), and Check 5e — they never report the same defect. Enforce the Truth Ownership Visibility Law (`framework/spec_writing_guide.md` §14.1 and §14.2 Anti-Pattern A): a non-owner unit must never enumerate volatile, unexported, or private internal parameters/fields of collaborating units.
   - For every attribute, parameter, field name, or payload structure cited by the candidate spec regarding a dependency unit, verify that it resolves to one of:
     - **Public Contract Anchor:** An exported public contract symbol or explicit schema property declared in the dependency's promises (for example `contracts.SpanAttr*`, standard exported protocol events, or public REST/gRPC response keys).
     - **Behavioral Specification:** A behavioral description that does not hardcode unexported parameter names (e.g. "records delegation goal and child execution identifier" instead of literal private parameter names `goal, role, child_run_id`).
   - **Transitive Penetration Check:** If the candidate spec cites internal fields or types of an indirect dependency (a unit not named by a consumer promise's `depends_on`), verify whether an explicit contract path exists. Citing internal fields of undeclared transitives without an exported contract anchor is a boundary violation.
   - If a spec mirrors or hardcodes unexported, private, or volatile internal fields of another unit:
     - **FAIL (P1: Shadow specification)** (actionable: replace private field enumerations with public contract anchors or behavioral specifications).

**Evidence:** read the promise regions the judgment depends on — the dependency unit's `## Promises` block from the session's read refs. Reports carry no dependency declarations; the tooling records the run's whole input surface (see §Step 8).

**PASS:** No contradictions across related units; acknowledged contract changes are declared; no shadow specifications found

**FAIL:** Contradiction found (actionable), shadow specification / private implementation leakage found (actionable), or unacknowledged contract breakage (needs_decision)

**Check method:** Target × related provider promises resolved in the target layer's promise scope (candidate/current layer for a candidate validate; published stable for a stable-only validate).

---

## Check 8 — Constraint alignment

**Purpose:** The candidate design must not violate global constraints or bound rules.

**Execution steps:**

1. Read the stable global rule set (`docs/specs/rules/stable/g_rule_*.md`) and each bound rule listed in `rule_refs`. Stable global rules apply to every current-layer unit by default and are not repeated in `rule_refs` (see `framework/spec_writing_guide.md` §5). Execute the two framework dependency prohibitions:
   - **Circular dependency prohibition** — run the mechanical promise-contract check first (`specflowctl validate promise-model --unit <name> --layer <candidate|stable>`): a promise contract cycle fails every unit that owns a promise on it, and a **unit dependency cycle** — two units each having a promise that depends on a promise of the other — also fails every unit on it. The promise graph is built from promise `depends_on` edges and is the authoritative contract cycle relation; unit edges aggregate every promise of one unit onto another, so two units can be mutually dependent through different promises while the promise graph stays acyclic. `deps` reports unit dependency cycles as violations and orders the units structurally over every promise dependency edge; it reports a separate **publication-order** relation over provider promises that are not yet published stable — the `[wait]` edges are the only publication-order blockers; a unit dependency cycle separately fails validate and stops promotion. The session still verifies its own unit's declared edges: read this unit's promise `depends_on` (required for the cross-unit work below anyway) and confirm each edge matches the graph the tool reports — a mismatch is a tool-input problem to report, not a judgment call. A contract cycle or a unit dependency cycle is a validate violation; analyze and present cycle-resolution guidance only when Check 15 reports one. `deps@all` reproduces the structural dependency order and cycle members (see `framework/verification_scope.md` §Dependency Analysis); `deps@{rule}` reports rule consumers. Graph build failures are fail-closed: an unreadable unit spec (permission, corruption) blocks the whole graph, so the promise-model check (mechanical Check 15) fails every unit with the failing file named in the Details — repair the file and re-run validate. A unit's missing or malformed `## Promises` section fails that unit and its consumers, not unrelated units.
   - **Layer-order prohibition** — for each applicable rule that records a repository layer order, resolve the order from that rule's recording and each unit's declared architecture layer from its spec truth (architecture section or design decision records); a promise `depends_on` edge from a lower-layer unit to a higher-layer unit is a violation. Units whose spec records no architecture layer are not judged by this prohibition.
2. Check the candidate design against each global rule and each bound rule:
```
   - Is every "must not" prohibition respected?
   - Is every "must" requirement satisfied?
```
3. **Cycle resolution guidance:** distinguish the two violations and the promotion diagnostic. A **promise contract cycle** (Check 15) is a spec violation: extract the shared contract into a rule (making the dependencies star-shaped), or re-draw unit boundaries so the coupled contract lives in one unit. A **unit dependency cycle** (Check 15, reported by `deps`) is also a spec violation: two units are mutually dependent through different promises (for example `a.p1 → b.q1` and `b.q2 → a.p2`), which leaves the promise graph acyclic but makes the units unorderable and their implementations mutually referencing; extract the shared contract into a rule or redraw unit boundaries. A **publication-order cycle** (`deps`) is the promotion diagnostic of a unit dependency cycle whose edges are all unpublished: every member waits for another member's unpublished contract, so none can be promoted first; fixing the unit cycle is the fix. Present the analysis to the user; the user decides the resolution, and the fix is applied only after explicit approval.
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

**Purpose:** Report the mechanical surface-association audit for this unit's declarations. The semantic halves of the former Check 9 live with their owning checks: whether each declaration participates in its item's behavior is verify Step 3 (`framework/unit_verify_checklist.md`) — a spec-vs-code judgment outside validate's spec-quality scope; shared-agreement single-source consistency and dependency-declaration accuracy are Check 7 step 7.

**Execution steps:**

1. Run `specflowctl surfaces` and confirm the mechanical `Surface associations` audit for the target passed. Read current and stable associations, directory expansion, and each unit's declared scope: every declared file resolves, and the derived associations agree with the declarations. Invalid paths are handled by the anchor checks.
2. Report the mechanical verdict: PASS when the audit passes and neither Check 6 nor Check 7 reported an association contradiction; FAIL when the audit reveals an anomaly (an unresolvable or inconsistent association), with the file and the contradiction named — the semantic judgment is routed to Check 6 or Check 7.

**PASS:** The association audit passes and no owning check reported an association contradiction.

**FAIL:** An association anomaly the audit reveals — P1 with the existing evidence fields. File overlap alone is never FAIL.

**Check method:** Mechanical association audit + consistency with the owning checks. See `framework/shared_judgments.md`.
## Check 10 — Clarity

**Purpose:** Read the unit spec and report what is unclear, underspecified, or internally contradictory. This is a single independent session, judged like every other validate check (report `PASS | WARNING | FAIL` plus the standard finding block) — there is no reader/verifier split and no closed-book reconstruction. It judges the Reader Contract (`framework/spec_writing_guide.md` §9) directly: whether the spec's human-readable part states the design clearly enough that its readers can understand it without guessing, and whether any statement conflicts with another.

**Reading scope:** the unit's complete spec — the main spec and every appendix (the same union as every other check). The check reads it directly; it reconstructs nothing.

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
- **Empty acceptance item set:** the spec is promise-model invalid — coverage requires every promise to be covered by at least one acceptance item (the Promise-model checks Coverage standard above), and a unit without a real `promises:` list fails the shape check — so the mechanical pre-pass and `gate-plan` reject the run before any session launches, with `specflowctl validate promise-model --unit <name> --layer <candidate|stable>` as the diagnosis entry. The clarity judgment never sees a promise-invalid spec.
- **Targeted runs:** `validate@{unit}:clarity` (or `:check-10`) executes the same single-session clarity check directly in the main agent session, without a coverage run or cache write; a P0/P1 finding must first be persisted with `gate-invalidate --check 10`.

---

## Step 9 — Write validate cache (tooling finalize)

When relationships are assigned or findings exist, after all local check sessions complete the independent final session re-verifies the extraction artifacts of Check 2 Step 1 and Check 5 FAIL findings (sub-check 5a step 8 / sub-check 5b step 5 / sub-check 5h step 8), checks only the assigned validate relationships without repeating local checks, and supplies the dispositions and relationship results the tooling's status closure consumes (the effective check-status map is tool-derived — the report carries no status lines). `gate-finalize` then decides the cache mechanically from that accepted synthesis:

==ATOM_BEGIN:cache_evidence_path_forms==
**Recording cache evidence — tool-generated:** gate cache entries are never agent-declared. `gate-finalize` records one entry per file of the run's input surface (the union of every coverage key's read refs and the cross-synthesis surface), each carrying the whole-file `hash` and the ordered content-defined chunk sequence of the current bytes, plus a per-check marker (check key + lens; failure status on failure records). Reports carry no `Dependency scope` lines; there is nothing for a reviewer to declare and nothing to validate against `read_refs`.

Spec objects that are resolved by name (a dependency unit's main spec or appendix, a rule file) are recorded as **logical references** instead of physical paths — `unit:{name}` for a unit main spec, `unit:{name}:appendix:{file}` for a unit appendix (the full appendix file base name without `.md`, e.g. `unit:auth:appendix:unit_auth_account_token_claims`), `rule:{id}` for a rule file. The run's own target files (the unit's own main spec and appendices; for a rule target, the candidate rule file and its stable sibling) and code files keep physical paths. A logical reference resolves at freshness time to the current-layer file (candidate first, stable fallback), so promoting the referenced unit or rule does not stale a cache whose recorded content is unchanged (see `framework/validation_cache.md` §Logical References).
==ATOM_END:cache_evidence_path_forms==

- **If all checks PASS (after re-verification):** the coverage run writes the validate cache per `framework/validation_cache.md` format:
  - `gate-finalize` creates `docs/specs/meta/validation/unit/{name}/` as needed
  - Collect dependency evidence for every file read during validation, including:
    - Main spec file
    - Every appendix file
    - All referenced files (rule_refs are already included)
  - `gate-finalize` assembles the cache from the accepted reports and the run input surface: one entry per input-surface file (whole-file hash + ordered chunk sequence) and one marker per executed check (check key = the agent check number; see `framework/validation_cache.md` §Format).
  - The `gate-finalize` write produces `validate_result.md` with `result: pass`, `target: candidate`, `mode: full`, and the complete input-surface evidence (whole-file hash + ordered chunk sequence per file).
  - Targeted runs (`:check-{n}` / `:{keyword}`) never write a cache, and a targeted run that FAILs deletes a pass cache (a failure record is kept — it is already blocking and is the recovery baseline) — any FAIL at any granularity means promote must not proceed — see `framework/validation_cache.md`

- **If any FAIL remains (after re-verification, full run, candidate target):** `gate-finalize` writes a failure record (`result: fail` + `blocking: true`, `mode: full`, `basis: full`, severity counts, findings body, and the per-check `status` map — `pass`/`fail` for every executed check; full runs re-execute local checks and may carry unchanged relationship judgments) — the failure-recovery baseline for `revalidate@{unit}`. Promote must not proceed. Proceed to Present Findings. (A stable-only full FAIL writes the same record shape — see §Stable-only mode.) After the findings are resolved, `revalidate@{unit}` (repair) re-checks the failed checks, persisted invalidated checks, newly affected checks, current keys the baseline never declared, and explicit `--rerun` overrides, then carries the passed checks over — each carried `pass` was executed as a complete independent judgment and its recorded content is unchanged (see §Delta re-run and §Failure handling by gate role in `framework/validation_cache.md`). During the fix iteration, targeted re-checks (`validate@{unit}:check-{n}` / `:keyword`) remain the lightweight local option.

## Extraction re-verification (Check 2 Step 1, Check 5)

Step 1 goal-means findings (Check 2), content-boundary findings (Check 2 Step 5 self-content; sub-check 5a step 9), uncovered-domain and orphan-item findings (sub-check 5a, step 8 artifact), promise-item contradictions (sub-check 5b step 5), and promise-content/uncarried-contract findings (sub-check 5h step 8) carry an extraction artifact. Before classification (cross synthesis), the independent final synthesis re-verifies each artifact with deterministic checks:

- Sub-agent claims a Check 2 Step 1 flag holds ("the flagged behavior serves no stated goal", "the described behaviors cannot meet the goal", "the behavior violates a non-goal", or "a smaller design surface would achieve the same goal") → re-read the goal or non-goal declaration and the flagged behavior or design surface; a quote that does not establish the claimed relationship — including a proportionality claim resting on preference rather than an establishable smaller surface — drops the finding
- Sub-agent claims "no item covers behavior subject X" → re-read the item set (the union of every item's `description` and `pass_condition`) and confirm X's quoted subject terms are really absent; a subject actually mentioned, or behavior variants split out of a covered domain (granularity violation), drops the finding
- Sub-agent claims "no designed behavior matches item X" (orphan item) → re-read the spec union (main spec body and every non-evidence appendix) and confirm X's quoted subject terms are really absent as a designed behavior; a designed behavior whose subject matches, or terms naming a scenario of a designed domain, drops the finding
- Sub-agent claims "no carrier states contract value Y" → grep the promises' `contract`/`text` for the quoted value; a value actually carried at comparable granularity drops the finding
- Sub-agent claims promise Y's `contract` conflicts with item X's `description`/`pass_condition` (5b step 5) → re-read the quoted promise field and item text; a value the item narrows to a legal subset, a promise range that contains the item's stricter bound, or an item that adds a stricter case without contradicting the promise drops the finding
- Sub-agent claims "Z is a behavior domain" (5a step 2), "Z is a promise narrative statement" (5h), or "Z is appendix contract content requiring a promise carrier" (5a step 6) → check the classification against the external-visibility boundary of `framework/spec_writing_guide.md` §15.2 (externally-observable behavior vs internal design detail); content reclassified as internal detail drops the finding
- Sub-agent claims "this passage is internal implementation / change history" (Check 2 Step 5) or "item X's subject is internal implementation" (5a step 9) → check the claim against §15 on its declaration-surface footing: re-read the content's own wording and the declared boundaries (this unit's carriers and the carriers of every promise provider unit). An internal-implementation claim is retained when §15.2 disposition 2 applies (the wording places the content on the internal side) or disposition 3 applies (a concrete implementation fact whose boundary the declarations leave undeclared — retained as `needs_decision`), and drops when disposition 1 applies (a declared consumer reads, is bounded by, or is affected by the content) or when the passage is current design expression carrying no concrete internal fact. §15.2 is the authoritative definition; this step cites it.

Re-verification failure → the cross result marks the finding `suppressed`; the affected check's tool-derived effective status is PASS if no retained finding supports FAIL. This runs before classification because a post-execution check re-run cannot detect a wrong-direction fix. Every suppression remains visible in the cross audit artifact with its location and reason; only retained findings enter the generated user report and counts.

## Delta re-run (revalidate@{unit})

Candidate targets, plus stable-only targets with a usable baseline — a delta re-run against a stable-only target whose confirmation cache is MISSING reports the missing-baseline message — "No usable confirmation baseline. Run the full `{command}@{target}` (confirmation check) first, or `specflowctl fork {--unit|--rule} {name}` to start a new round." — and stops (see `framework/verification_scope.md` §Delta Runs → Layer applicability). Triggered by the user when the cache is STALE or BLOCKED (a failure record). Follow §Delta Runs in `framework/verification_scope.md`:

1. **Preconditions** — the existing cache must have `mode: full` and one of the two baselines: `result: pass` (stale-cache recovery) or `result: fail` + `blocking: true` (failure-record recovery, §Failure recovery — the record written by a candidate full-run FAIL or by a delta FAIL). A MISSING cache has no usable baseline — run the full command instead.
2. **Plan (change review)** — run `specflowctl gate-plan --gate validate (--unit {name} | --rule {id}) --target {candidate|stable} --mode delta` (or `--mode repair` for a failure record). The planner computes the mechanically detected change set (ordered bidirectional chunk diff over the recorded input surface; new physical inputs join as added files), schedules one `review` session carrying the change set and the standing conclusions, and mechanically forces failed and persisted-invalidated checks, current keys the baseline never declared, and explicit `--rerun` overrides. It reports the change set, the forced keys, the carry candidates, and any degradation or full-scope statement. After a targeted P0/P1, the coordinator runs `gate-invalidate` immediately; the planner reads that state automatically and never carries the contradicted judgment. A cache without the required chunk evidence is not current-format and is treated as no baseline — the planner refuses the delta/repair and directs a full run. A failure record whose per-check status map is absent, invalid, or inconsistent with its judgment baseline covers the full scope. The review decides the remaining scope; the executor does not re-derive it.
3. **Review and execute** — generate the review mission with `specflowctl gate-mission --run <run_id> --keys review --format prompt` and submit its report with `specflowctl gate-submit --run <run_id> --session review --keys review --report PATH`. `Review result: accept` keeps the planned carry set; `Review result: recheck` with `Recheck: <keys>` adds those conclusions' coverage keys to the run (re-mission and execute them; each re-run session uses the standard checklist with the change set as context); `Review result: escalate-full` abandons the run — start a full run instead. Then execute the planned coverage keys (protocol: `framework/verification_scope.md` §Coverage Model), submitting each re-run report with `specflowctl gate-submit --run <run_id> --session <session_id> --keys <keys> --report PATH`. Any P0/P1 finding during the re-run makes the finalize write a **failure record** (`result: fail`, `blocking: true`, severity counts, findings body, `mode: full`, `basis: delta`, and the per-check `status` map — `fail`/`pass` for the re-run checks, `carried` for the carried-over ones, derived by `gate-finalize` from the accepted session verdicts). Stop, present findings (same as full FAIL — Check 2 Step 1 and 5a/5b/5h findings re-verified per §Extraction re-verification before presentation).
4. **On PASS** — `gate-finalize` rewrites `validate_result.md` with `mode: full`, `basis: delta` (or `basis: repair` when recovering from a failure record), `target: candidate` (stable-only target: `target: stable`), a fresh `timestamp`, the recorded review (`reviewed_change_set`, `review_result`, `review_session`, `review_recheck` when named, and `review_declined` when the review left carry candidates un-re-run), and a **complete** `files` list: fresh `hash` + `chunks` evidence for the re-run paths and the plan-time snapshot's evidence for the carried-over ones, merged by the tooling. Logical references (`unit:{name}` / `unit:{name}:appendix:{file}` / `rule:{id}`) are carried over the same way.

---

## Present Findings

Advisory findings (Check 1 step 3 hygiene WARNING) are presented for awareness only — they enter neither the batch group nor the decision group, need no decision, and do not block the flow. They are presented on their check line's reason even when all checks PASS. Advisory findings are never emitted as bracketed findings — validate grades findings P0/P1 only, and a retained P2/P3 finding rejects the run's synthesis at `gate-finalize`.

### Batch classification (validate)

When FAIL items exist, the main agent classifies each finding into a **batch group** or a **decision group** before presenting. Classification uses each FAIL's resolution type (actionable / needs_decision) and the check definition — it adds no new analysis; for batch group candidates it only runs lightweight assertion re-verification (see Assertion re-verification below).

**Batch group eligibility:** A finding enters the batch group only if its fix is fully determined by an objective standard — no interpretation of design intent is required. The batch group is limited to these fix types:
- Check 1: missing required frontmatter fields (standard: the required fields list)
- Check 1: rule_refs / appendix references to non-existent files (standard: file existence)
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

Suppressed findings from the extraction re-verification (Check 2 Step 1, Check 5) are dispositioned in the cross report's audit artifact (see §Extraction re-verification) and do not appear in the report or its counts.

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

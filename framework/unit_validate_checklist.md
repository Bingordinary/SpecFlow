# Validate Checklist

## Overview

When an agent executes `validate@ {unit}`, it uses the 8 checks defined in this file. This file is referenced by `framework/concepts.md` §3 — the agent reads this file at validate time, not proactively.

## Prerequisite — Read all unit files

Before executing any check, the agent must read the complete unit content:

1. Read the main spec: `docs/specs/units/candidate/unit_{unit}.md`
2. Glob all candidate appendix files: `docs/specs/units/candidate/appendix/unit_{unit}_*.md`
3. For each appendix, read its frontmatter. Skip files with `status: exempt` or `status: retired`.
4. Read the content of every non-exempt, non-retired appendix file.

The unit's complete spec is the union of the main spec and all non-exempt appendix files. All checks that follow operate on this union.

## Mode Selection

| Trigger | Mode | What to execute |
|---------|------|-----------------|
| `validate@ {unit}` | full | All 8 checks + cross-check. Quality checks are holistic — always runs full. |
| `validate@ {unit}:check-{n}` | targeted | Single check `{n}` only. User explicitly chooses focus. Does not write a cache. |
| `validate@ {unit}:{keyword}` | targeted | Match keyword to check name (e.g., "design" → Check 2, "scope" → Check 3). User explicitly chooses focus. Does not write a cache. |

**Keyword domain:** validate keywords resolve to check names — `structure` (Check 1), `design` (Check 2), `scope` (Check 3), `evidence` (Check 4), `acceptance`/`coverage` (Check 5), `affects` (Check 6), `cross-unit` (Check 7), `constraint` (Check 8). A keyword matching no check name is a no-match — ask the user for clarification.

**Output:** Targeted runs report only the executed check(s) and note "This was a targeted check — no cache was written. Run `validate@ {unit}` for a complete validation."

### Stable-only mode

When no candidate spec exists (validate against stable), run the same 8 checks + cross-check against the **stable** content:

1. Read the stable main spec: `docs/specs/units/stable/unit_{unit}.md`
2. Glob all stable appendix files: `docs/specs/units/stable/appendix/unit_{unit}_*.md`, read every non-exempt, non-retired appendix (same skip rules as the candidate path)
3. Run all 8 checks + cross-check against the stable content — Checks 6/7/8 are the live part: referenced files, dependency-unit contracts, and rules may have changed since promote, so the stable content may no longer hold (e.g. a new rule now prohibits something the stable design does)
4. **PASS** → write the validate cache with `target: stable` (confirmation state consumed by `fresh@stable`; `mode: full`, `hash` + `deps` evidence, same procedure as Step 9)
5. **FAIL** → delete the existing validate cache if present, do not write a cache, present the findings, and recommend forking the unit (`specflowctl fork --unit <name>`) to reconcile the stable content with the changed dependency or rule. Do not edit the stable spec directly — promote is the only operation that writes stable files

The stable confirmation cache is read-only state: it grants no promote eligibility (stable has no gate). Delta re-runs (`revalidate`) apply to stable-only targets when the confirmation cache exists with `result: pass` (STALE recovery); a MISSING or BLOCKED stable cache needs the full confirmation run — see `framework/verification_scope.md` §Stable-only Targets and §Delta Runs → Layer applicability.

## Execution Rules

- **Subagent permissions:** validate executes in an independent read-only sub-agent session — the sub-agent may inspect file content, search text by pattern, and locate files by name pattern. Must NOT modify files, execute commands, or delegate to other agents. The sub-agent prompt is assembled per the validate shape of `framework/verification_scope.md` §Sub-agent Prompt Assembly; targeted runs (`:check-{n}` / `:{keyword}`) execute directly in the main agent session instead (see §Targeted Runs in `framework/verification_scope.md`).
- Each check reports **PASS** or **FAIL** with a reason.
- On FAIL, the agent must identify **which information sources contradict each other** (e.g., "spec body describes auto-retry logic but no acceptance item covers it"). The fix is recorded in the FAIL reason.
- Resolution types:
  - **actionable** — A concrete repair can be made inside the current candidate spec without user judgment.
  - **needs_decision** — Requires user input (unclear intent, missing decision, or external dependency). Stop and ask.

## Output Format

==ATOM_BEGIN:report_skeleton==
## Unified Report Skeleton

All quality-gate reports (validate, verify, review) share the same report skeleton below. The header lines (`Result`, `Blocking promote`, `Key counts`) and the `Next step` line are identical across commands; only the body content and the Findings section entries' detail lines are command-specific and defined in each checklist file.

```
────────────────────────────────────────────
{command}@{target} · {mode} · {layer}
Result: PASS | FAIL
Blocking promote: yes | no
Key counts: Findings: N (P0: a | P1: b | P2: c | P3: d)
────────────────────────────────────────────
{body}
────────────────────────────────────────────
Dependency scope:
  {check key}: {file}: {declaration}   # per executed check; declaration = section heading text, line ranges, or "all"
────────────────────────────────────────────
Findings:
  [{severity}] {location} — {issue} (actionable | needs_decision)
  {command-specific detail lines, indented}
────────────────────────────────────────────
Next step: {concrete next command with reason, or "None"}
────────────────────────────────────────────
```

**Field definitions:**

- `{command}@{target}` — the command and target that produced this report, e.g. `validate@user_auth`, `verify@user_auth`, `review@user_auth`. Commands: `validate`, `verify`, `review`. Targets: unit or rule name.
- `{mode}` — `full` for full runs; `targeted (user requested: {keyword})` for targeted runs; `delta` for incremental re-runs (`revalidate@{target}` / `reverify@{unit}` / `rereview@{unit}`).
- `{layer}` — the spec layer checked: `candidate` | `stable`.
- `Result` — `PASS | FAIL` for all three commands. The gate is decided by severity: FAIL means P0/P1 findings exist. validate grades findings P0/P1 only, so any validate FAIL is a P0/P1 finding; verify/review FAIL means P0/P1 mismatches/findings exist.
- `Blocking promote: yes | no` — `yes` when P0/P1 findings exist (the run FAILs); `no` otherwise. Valid for all three commands.
- `Key counts: Findings: N (P0: a | P1: b | P2: c | P3: d)` — N is the total finding count, a/b/c/d the count per severity. validate grades findings P0/P1 only, so its P2/P3 counts are always 0; verify's blocking mismatches equal the P0/P1 counts and non-blocking mismatches equal the P2/P3 counts. Command-specific summary numbers (validate's failed checks and advisory findings, verify's coverage, review's suppressed-by-spec count) appear in the body.
- `{body}` — command-specific content defined in this file's body format section (validate: one line per check; verify: Items / Scope / Integrity / Coverage / first-principles divergence analysis; review: Architecture assessment and suppressed findings).
- `Findings:` — one entry per finding in the unified format `[{severity}] {location} — {issue} (actionable | needs_decision)`. `actionable` — a concrete repair can be made without user judgment (verify: direction spec_gap/code_gap; review: a determined recommendation); `needs_decision` — requires user input or a design decision before the fix can be made (verify: direction needs_design/blocked; review: architecture trade-offs). Command-specific detail (verify's suggested direction, review's spec_context/recommendation, validate's contradicting information sources) appears as indented detail lines under the entry. Findings are grouped into the batch group and decision group defined in this file's batch classification section when this file defines one; flat when this file defines no batch classification or grouping is inactive.
- `Dependency scope:` — one line per check the run executed: `{check key}: {file}: {declaration}`. `{check key}` is the command's check identifier (validate: `check-{n}`; verify: the acceptance item id; review: the batch/file dimension). `{declaration}` is the section-region heading text the check's judgment read (e.g. `Description`, `Testability / Acceptance Criteria`; the frontmatter region is `frontmatter`), 1-based closed line ranges (e.g. `120-180,300-320`), or `all` when the judgment covered the whole file. In full runs, every read-only subagent reports this scope for the checks it executed and the main agent carries it over verbatim; in single-executor flows, the executor reports its own scope for the checks it executed. The main agent uses it when writing the validation cache — section headings become `--section` declarations, line ranges become `--ranges`, `all` is a whole-file declaration — and records the per-check breakdown in the cache's `checks` mapping (see `framework/validation_cache.md` §Format → Per-check evidence). Delta runs report the scope of the re-run checks only — carried-over checks are not re-executed and get no new declaration (see `framework/verification_scope.md` §Sub-agent Prompt Assembly Check scope). Targeted runs may omit it.
- `Incremental scope:` — delta runs only (mode `delta`). One line per re-run check in the run's own structure (e.g. validate: "check-5 (acceptance coverage & correctness): re-run — section `Description` of the unit's own spec changed"), followed by a line declaring the carried-over checks ("checks 1-4, 6-8: carried over — their dependency evidence is unchanged") and the cross-check result (unit targets — rules have no cross-check). The scope is mechanism-derived from the cache's per-check evidence: stale regions map to the checks that declared them (see `framework/verification_scope.md` §Delta Runs). The incremental scope must be reported before execution begins (the user must see what will be re-run and what will be carried over) and again in the final report.
- `Next step:` — the concrete command to run next with its reason; `None` when nothing further is needed. Guidance: fixes applied → "fixes applied — re-run the target-appropriate re-check command (`validate@{target}:check-{n}`; unit targets also `verify@{target}:{keyword}` / `review@{target}:{keyword}`) to confirm"; all gates green → "if the design is finalized, run `promote@{target}`"; needs_decision → "awaiting your decision on {item}".

**Targeted runs:** end the report with the command's targeted note ("This was a targeted check — no cache was written. Run `{command}@{target}` for a complete ...") after the `Next step` line.
==ATOM_END:report_skeleton==

### Body format (validate)

One line per check, numbered as in this file:

```
1. Structural integrity: PASS | WARNING | FAIL — reason
2. Design soundness: PASS | FAIL — reason
3. Scope integrity: PASS | FAIL — reason
4. Evidence-driven vs design-driven consistency: PASS | FAIL — reason
5. Acceptance coverage & correctness: PASS | FAIL — reason
  5a. Coverage completeness: PASS | WARNING | FAIL — reason
  5b. Content alignment: PASS | FAIL — reason
  5c. Internal consistency: PASS | FAIL — reason
6. Affects-source validity: PASS | FAIL — reason
7. Cross-unit consistency: PASS | FAIL — reason
8. Constraint alignment: PASS | FAIL — reason
```

After the check lines, report the cross-check line (full runs only) and the counts:

```
Cross-check: 3/3 PASS — per-check results; contradictions are presented as findings
Failed checks: N | Advisory findings: K
```

**Counting rules:**
- `Findings: N (P0: a | P1: b | P2: c | P3: d)` — N is the total number of distinct findings across all FAIL checks; a/b/c/d the count per severity. validate grades findings P0/P1 only — P1 is the contract-decided default, P0 requires severity confirmation per `framework/severity_policy.md` §9 (see Severity check below) — so `c` and `d` are always 0. In targeted runs, only executed checks are counted.
- `Failed checks` is the number of FAIL checks among executed checks, shown in the body's check lines. WARNING is not a failed check.
- `Advisory findings` (Check 2 Step 4 taste-level P2/P3) are presented on their check line's reason and counted separately as `Advisory findings: K` in the body. They are never counted in `Findings` and never affect `Failed checks`.
- The same counts are reused in the Present Findings summary (`Findings` N = batch group items + decision group items).

**Multi-finding enumeration:** When a FAIL reason contains multiple distinct findings, list each finding as an indented numbered sub-line under the check line, in the unified finding format `[{severity}] {location} — {finding} (actionable | needs_decision)`. Each entry carries a location reference (the contradicting information sources, per Execution Rules), the finding statement, and its resolution type:

```
5a. Coverage completeness: FAIL — 3 findings
  5a-1. [P1] {location} — {finding} (actionable)
  5a-2. [P1] {location} — {finding} (actionable)
  5a-3. [P1] {location} — {finding} (needs_decision)
```

A check with a single finding keeps the existing one-line reason format.

When findings mix resolution types (within one check or across checks), the report presents each finding with its own resolution — a `needs_decision` finding stops the flow and requires user input per Execution Rules.

**Evidence discipline:** every check line's reason must reference the spec content the verdict covers (section title, item id, appendix name). A PASS with no reference means the check did not run — report FAIL with "no evidence basis".

---

## Check 1 — Structural integrity

**Purpose:** Verify the file is parseable and all required fields exist, as a prerequisite for all subsequent checks.

**Execution steps:**

1. Read `docs/specs/units/candidate/unit_{unit}.md` and all non-exempt appendix files (see Prerequisite)
2. Verify required frontmatter fields: `id`, `version`, `unit_refs`, `rule_refs`. The spec layer is encoded by the file path (`docs/specs/units/candidate/` vs `docs/specs/units/stable/`) — no `layer` frontmatter field is declared (see `framework/spec_writing_guide.md` §3)
3. Verify `acceptance_item_set` exists with at least one item. Each item must have: `id`, `description`, `verification_type`, `verification_surface`, `implementation_surface`, `verification_method`, `pass_condition`, `runnable`. `implementation_surface` may be the placeholder `<pending>` during design-first rounds (the path is not yet known); the placeholder must be exactly `<pending>` — variants are reported for correction. A leftover `<pending>` is a MISMATCH in verify (Step 6), so it must be replaced with the real path once the implementation exists
4. Verify all `unit_refs` point to existing spec files (bare name, e.g. `agent`). Resolve by searching candidate directory first (`unit_{name}.md`), then fall back to stable (`unit_{name}.md`).
5. Verify all `rule_refs` point to existing rule files (global or bound)
6. Verify any appendix files referenced in the spec body exist at the expected path
7. **Prose-path hygiene check (WARNING):** Verify that prose sections (Description, Responsibility, and any other narrative sections) do not contain code file paths:
   - Scan narrative text for strings matching source-code file path patterns (backtick-enclosed or bare strings containing `/` and a source-code file extension like `.go`, `.ts`, `.py`, `.js`, `.java`, `.rs`, `.cs`)
   - Exclusions:
     - Structured fields: `implementation_surface`, `affects.files` (intentional)
     - Framework governance paths (`framework/`) and validation cache paths (`docs/specs/meta/`) — describe the governance system itself
     - File paths inside code-block examples serving as illustrations
   - If code file paths are found in prose → WARNING with quoted path, section name, and line reference
8. **Appendix frontmatter check:** For each non-exempt, non-retired appendix file, verify:
   - `unit` frontmatter field matches current unit name
   - `status`, when present, is one of `active` (or absent), `exempt`, `retired` — any other value is FAIL
9. **Appendix path check:** Verify each appendix file's path matches the convention: `docs/specs/units/candidate/appendix/unit_{unit}_{name}.md`
10. **Layer-prefix path check (FAIL):** Scan the main spec body and every non-exempt appendix for layer-prefixed spec paths that break after promote or mispoint during an active candidate round. Unlike step 7, there is no code-block exemption: paths inside code-block examples are flagged the same as prose.
    - Absolute forms: `docs/specs/units/candidate/`, `docs/specs/units/stable/`, `docs/specs/rules/candidate/`, `docs/specs/rules/stable/`
    - Relative forms: `candidate/`, `stable/`
    - Reference appendix files and other specs by concept name or file name (e.g. `unit_auth_account_token_claims`) instead — appendix file names do not encode layer, so the reference stays valid before and after promote
    - Structured field exemption: `implementation_surface`, `affects.files`, `affects.appendices`, `affects.dependencies` values may contain a stable-layer spec path (it stays valid after promote); a candidate-layer spec path in any structured field is invalid (promote deletes candidate files)
    - If layer-prefixed spec paths are found → FAIL with quoted path, section name, and line reference
11. **Section structure check (FAIL):** the unit's own main spec must be splittable into section regions by mechanism — the per-check dependency declarations (`--section`) fail closed otherwise (see `framework/validation_cache.md` §Structural Region Dependencies). Run `specflowctl gate-evidence --file <spec> --sections` and verify:
    - The spec has at least one `##` heading (frontmatter region + at least one section region)
    - No `##` heading text is duplicated (a duplicated heading cannot be located unambiguously — fail closed)
    - The frontmatter region (before the first `##` heading) contains only the YAML frontmatter block, the `#` document title, and blank lines — any other content there is stray prose that belongs to no region
    - Fix direction: restructure the spec per `framework/spec_writing_guide.md` §13 (section regions)
12. **Region structure semantics check (FAIL):** the section regions must match the content's semantic structure — the region mechanism's trust (delta scope, cache freshness) is only as correct as the split. Run `specflowctl gate-evidence --file <spec> --sections` and, reading the spec itself, verify:
    - **The acceptance_item_set region covers every real item** — the region runs from the exact `acceptance_item_set:` marker line to the next top-level heading outside a code fence; a stray top-level `#` line inside the item list truncates the region, and items after the truncation would sit outside the declared dependency (a false-fresh risk). Distinguish real items from fenced example blocks — a fenced `- id:` example is content, not a truncated item (the mechanical `specflowctl validate` Check 9 covers the exact-marker and heading-format preconditions; the semantic coverage judgment is this step's)
    - **Fenced code blocks are content** — `##`-like lines inside ``` / ~~~ fences must not split regions; when a fence would visually span a section boundary, the spec needs restructuring (a fence cannot cross `##` headings — close the fence before the next heading)
    - **Every region the run will declare is locatable** — for each section the checks will declare (run `--section <heading>` probes), the heading resolves uniquely. A declaration that cannot be located fails closed at cache-write time; this step surfaces it at validate time
    - Fix direction: restructure the spec so the split is semantically clean (cohesion per `framework/spec_writing_guide.md` §13); do not fall back to whole-file declarations as a workaround
13. **Version Notes check (FAIL):** verify the unit's own main spec complies with the Version Notes convention (see `framework/spec_writing_guide.md` §14). Applies to candidate specs only: stable-only runs skip it (stable content is consensus — the §14 Scope migration rule leaves the section to the unit's next candidate round), and retiring candidates skip it (the spec is being removed, not archived):
    - Existence and position: the spec must contain a `## Version Notes` heading as its first `##` heading, immediately after the `#` title; a missing heading or a heading in any other position is a FAIL
    - Lifecycle compliance: the section contains at most two entries — the top entry's version must equal the frontmatter `version` field, followed by at most one line summarizing the previous version; a section with more entries (an untruncated changelog carried over from the stable copy) is a FAIL
    - Content discipline: entries record design-decision-level changes (behavior, contract, boundary, config semantics) — implementation details, typo fixes, or formatting edits belong to VCS history, not the section
    - Fix direction: restructure the spec per `framework/spec_writing_guide.md` §14 (Version Notes)

**PASS:** All format constraints satisfied

**WARNING (step 7):** Code file paths detected in prose sections — relocate to `implementation_surface` or `affects.files`, or convert to a concept name reference

**FAIL:** Any missing field, reference to a non-existent file, or layer-prefixed spec path in prose or structured fields (actionable)

**Check method:** Unidirectional existence check (the only check that does not cross-reference, as it is the prerequisite)

**Communication note:** When suggesting Check 1 to a user, describe it as "structural integrity — verifies file structure and reference existence without evaluating design quality."

**Retiring unit:** when the candidate main spec declares `status: retired` (see `framework/spec_writing_guide.md` §8 Unit Retirement), the unit is being removed from stable. The acceptance item set is not required (agent Check 5's acceptance checks are skipped for the retired spec), and retiring appendices are skipped like exempt ones. Check 1 still verifies the required frontmatter fields. The retiring spec's own references (`unit_refs`, `rule_refs`, version pins, appendix and evidence references) are exempt from the mechanical checks — they disappear with it. The mechanical `specflowctl validate` checks use their own numbering (Check 1 frontmatter, Check 2 acceptance items, Check 3 anchor integrity, Check 4 reference integrity, Check 5 appendix files, Check 6 version consistency, Check 7 layer-path check, Check 8 dependency cycle check, Check 9 region locatability); of these, a retiring spec skips Check 2, 3, 4, 6, 7, 8, and 9, while Check 1 still applies and Check 5 keeps its appendix-level exempt/retired skipping. `specflowctl promote` also skips its reference checks for a retiring unit. Reference protection: a unit that is being retired must not be referenced by any other current-layer unit's `unit_refs` — the mechanical `specflowctl validate` Check 4 rejects such a reference; the agent must also report it via agent Check 7 (cross-unit) with `needs_decision` resolution until the referrer drops the reference.

---

## Check 2 — Design soundness

**Purpose:** Evaluate whether the design itself is correct and reasonable — not just whether it is well-documented — and whether the spec satisfies the full `framework/spec_writing_guide.md` §9 Authoring Baseline: the ten expression points are made clear (a reader who never sees the code can restate the design), the design decisions are closed, so the downstream executor is never forced to choose, and any intentionally unmade decision declares its boundary. The subagent must actively reason about the design, not passively verify documentation completeness. Appendix content describing design decisions, API contracts, component trees, or data types is part of the unit's design and must be included in this analysis.

**Execution steps:**

**Step 1 — Goal-means analysis**
- Read the unit's goal and scope from the main spec AND appendix files
- For each major behavior described (in main spec or appendices): does it demonstrably serve a stated goal? If a behavior cannot be traced to any goal → flag (possible over-engineering)
- Reversely: is the goal achievable by implementing all described behaviors? If implementing everything still does not meet the goal → flag (design gap)
- Check whether any behavior violates a stated non-goal (e.g., non-goal says "no multi-tenancy this round" but the behavior describes tenant isolation)
- **Proportionality check:** Is the design complexity proportional to the stated goal? If the same goal could be achieved with significantly less design surface area → flag (possible over-engineering)

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

**Step 4 — Design decision taste level**
Assess only the design decisions the spec has actually recorded (design decision records, architecture descriptions, component trees, data types in main spec and appendices). Evaluate taste-level quality of those recorded decisions:
- Are module/component boundaries cut at the natural seams of the stated behavior domains?
- Does each recorded responsibility stay single-purpose?
- Are extension landing points explicit where the spec records future-work expectations?
- Do the recorded decisions follow the repository's established engineering patterns?

Output P2/P3 advisory findings — these do NOT affect the PASS/FAIL verdict and do not block promote. If the spec records no design decisions (no architecture section, no design decision records, and no appendix design content) → report "Step 4: N/A (no recorded design decisions)" and proceed. This step must not invent architecture the spec does not record; it evaluates only what is written. Objective design defects remain Step 3's territory.

Before presenting advisory findings, confirm each P2/P3 grading per `framework/severity_policy.md` §9: read the appendix or body section the finding judges and any section its impact claim depends on, beyond the section it was graded on (§9.5 Execution Rules, rule 2), verify the §9.3 boundary, and record `confirmed` or `adjusted: {Px} → {Py}` with evidence. Advisory gradings are judgment-based and never contract-decided, so all of them are in scope.

**Step 5 — Authoring Baseline verification**
Verify that the spec satisfies the full `framework/spec_writing_guide.md` §9 Authoring Baseline in two layers: the ten expression points are made clear (a reader who never sees the code can restate the design), and every implementation-affecting decision is closed. The downstream executor must not be forced to choose.

- **Input discipline:** the spec text is the ONLY input (main spec + non-exempt, non-retired appendices). Do not fill spec gaps with implementation knowledge from this session — if the spec omits a point or a decision, the gap is real and must be reported. Reading the implementation defeats this check's purpose: a spec that only makes sense with code knowledge forces the downstream executor to choose, which is exactly what the baseline forbids.

**Layer 1 — Expression points (the "must make clear" list):** For each of the ten baseline points — (1) the intended user, actor, or caller; (2) the unit responsibility and why the unit owns it; (3) the entry point or trigger; (4) the normal path from input to result; (5) the boundaries crossed on that path; (6) the data, state, or durable truth each step reads or writes; (7) the owner of each read/write responsibility; (8) the output artifact or observable result; (9) the way failures or unavailable dependencies are exposed; (10) the verification surface and success condition — can the reader determine the answer from the spec alone? A point that is not made clear is a FAIL (actionable: express it in the spec).

**Layer 2 — Decision closure (the "must close" list):** Verify that the spec closes every implementation-affecting decision: which object owns a responsibility, which entry point starts the behavior, where state or durable truth lives, how ordered steps connect, how boundary failures are reported, what the result shape means, how acceptance proves the stated responsibility.

- For each of the seven decisions: can the downstream executor determine the answer from the spec alone, without making a choice?
  - For evidence-driven acceptance items (Step 2 waiver), the closure source is the evidence appendix: verify it records the item's behavior domain as directly readable behavioral truth (per `framework/spec_writing_guide.md` §3 `evidence_appendix_ref`), not only background, motivation, or patch notes.
  - The "how acceptance proves the stated responsibility" decision is covered by Check 5's acceptance quality review — here, only confirm the acceptance items exist and can prove the stated responsibility.
- If a decision is intentionally not made, the spec must state that boundary and explain why (per `framework/spec_writing_guide.md` §9, the "If a decision is intentionally not made" rule). An open decision without a stated boundary is a FAIL.
- Granularity: verify closure, not exhaustiveness — the spec must not be inflated into an implementation manual. Coverage obligations are limited to formal behavior domains (see Check 5a Step 2 extraction premise); narrative elaboration in the body does not add coverage obligations.

**FAIL:** any of the ten expression points is not made clear, or any of the seven decisions is left open AND not explicitly bounded with a reason (actionable: express the point, or record the decision / declare the boundary; needs_decision when recording it requires user input — Execution Rules "missing decision")

**Step 6 — Verdict**
- PASS: goal-means aligned, per-item rationale documented (evidence-driven items waived per Step 2), all ten §9 expression points made clear, all seven §9 decisions closed or explicitly bounded, no critical flaws found
- FAIL: specific findings reported

**Check method:** Content reasoning + adversarial analysis + taste-level assessment + authoring baseline verification (the subagent makes active engineering judgments)

---

## Check 3 — Scope integrity

**Purpose:** The declared scope, non-goals, and boundaries must be clear and internally self-consistent.

**Execution steps:**

1. Is the unit's goal and responsibility scope clearly stated?
2. Are first-round non-goals and boundaries explicitly defined?
3. Are dependencies, rule bindings, and ownership boundaries explicit?
4. **Self-consistency check (main spec):**
   - Do the goals and described behaviors agree? (goal description scope matches behavior scope)
   - Do non-goals conflict with any described behavior? (non-goal says "not doing X" but behavior describes X)
   - Are the boundaries respected by the behavior descriptions? (e.g., boundary is "client-side validation only" but behavior describes server-side logic)
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

**Purpose:** The spec body and acceptance items must cover each other bidirectionally, match semantically (5b), and contain no internal contradictions (5c). Acceptance items must also have falsifiable pass_conditions (5e), actionable descriptions (5f), and coupled pass_condition/description pairs that add value (5g).

**Execution steps:**

### Sub-check 5a — Coverage completeness

**Purpose:** Every behavior domain in the spec body and appendices must have at least one corresponding acceptance item, and the item's surface fields must be consistent with the behavior type. Granularity baseline: behavior domains as defined in `framework/spec_writing_guide.md` §Acceptance Item Granularity — one item = one behavior domain with its full scenario set (happy path + error paths + boundary cases). Enhanced from the original forward coverage check to a bidirectional check.

**Execution steps:**

1. **Coverage input source:** A behavior is covered when any acceptance item describes it in its `description` (Given/When/Then scenarios) OR constrains it in its `pass_condition`. The coverage judgment input is the union of `description` and `pass_condition` — a behavior constraint that already appears in some item's `pass_condition` counts as covered, consistent with sub-check 5g (which requires `pass_condition` to carry constraints beyond `description`).
2. **Extraction premise (shared with sub-check 5h):** Behavior-domain extraction targets only formal behavior declared in the spec body and appendices — a behavior subject (endpoint, function, state machine, or flow entry point) together with its behavior semantics. Non-constraining narrative (design discussion, illustrative examples, motivation, variant elaboration) is NOT a behavior domain source and does not create coverage obligations. Extract all behavior domains at the granularity defined in `framework/spec_writing_guide.md` §Acceptance Item Granularity: group behavior variants around one behavior subject into one domain (error paths, boundary cases, and state transitions of the same subject are scenarios of that domain, not separate domains); do not split scenarios of the same domain into separate coverage requirements.
3. For each behavior domain, verify at least one acceptance item covers it (using the union input from step 1)
4. For each covered domain, verify the item's `implementation_surface` and `verification_surface` are consistent with the behavior's nature (e.g., REST API behavior should have surface `api`, not `db`)
5. If a behavior domain has no acceptance item → flag (possible untested behavior)
6. **Appendix behavior coverage check:** Extract all behavior domains, API contracts, data type definitions, and state machine transitions from appendix files. For each, verify there is at least one acceptance item in the main spec covering it. If an appendix describes contract or behavior content that has no corresponding acceptance item → **FAIL (actionable)** — the acceptance item set is the complete formal behavior carrier (see `framework/spec_writing_guide.md` §4), and contract content without item coverage is invisible to the cross-unit consistency check of every dependent unit. If appendix content directly contradicts an acceptance item (e.g., appendix says "timeout: 30s", item says "respond within 5s") → FAIL (actionable)
7. **Over-splitting detection (reverse check):** If multiple acceptance items satisfy the same behavior domain judgment (same behavior subject + same `verification_surface` + same `implementation_surface` + same `verification_type`), they are merge candidates → WARNING recommending a merge into one item. Merge method: keep one item id, delete the rest — the surviving id's process evidence stays valid. Items differing in `verification_type` are legitimate splits, not merge candidates.

**PASS:** All behavior domains (main spec + appendices) have corresponding items with appropriate surface fields; no merge candidates found

**WARNING:** Merge candidates (over-split acceptance items)

**FAIL:** Uncovered behavior domain or surface type mismatch (actionable); appendix contract/behavior content without item coverage (actionable); appendix-main spec contradiction (actionable); contract statement without carrier coverage (5h, actionable); item violating the Contract Substance Baseline (5i, actionable)

**Check method:** Spec body + appendices × acceptance item set bidirectional cross-reference (body → items for coverage; items → body for over-splitting detection)

---

### Sub-check 5b — Content alignment (NEW)

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

**PASS:** No contradictions found

**FAIL:** One or more contradictions found, with quoted evidence (actionable)

**Check method:** Spec body × acceptance item pass_condition — semantic cross-reference with quoted evidence

---

### Sub-check 5c — Internal consistency (NEW)

**Purpose:** Detect contradictions between acceptance items within the same spec. Two items targeting the same verification surface must not describe contradictory requirements.

**Execution steps:**

1. **Group by `verification_surface`:** items sharing the same surface are likely checking the same endpoint/module
   - Compare their `pass_condition` texts for contradictions
   - Example: item A says "returns 201", item B says "returns 200" for same API → conflict

2. **Group by `affects.files`:** items referencing the same implementation file
   - Check if their behavioral descriptions conflict
   - Example: item A says "write requires auth", item B says "write is public" → conflict

3. **Cross-item value/assumption check:**
   - Detect obvious numeric contradictions (one says <100ms latency, another says <5s for same operation)
   - Detect logical contradictions (one says "enabled by default", another says "opt-in only")

**PASS:** No conflicts found

**FAIL:** One or more conflicts found, with quoted evidence from both items (actionable)

**Check method:** Cross-item cross-reference by verification_surface and affects.files

---

### Sub-check 5d — Description format compliance

**Purpose:** Verify that each acceptance item with `verification_type: testable` uses Gherkin-style Given/When/Then format in its `description`, as required by `framework/spec_writing_guide.md` §Gherkin-style Description Convention.

**Execution steps:**

1. For each acceptance item in scope:
   - If `verification_type` is `testable`, read the `description` field
2. Check that the description contains at least one `Given`…`When`…`Then` sequence (case-insensitive pattern: lines starting with `Given`, `When`, `Then` in order)
3. Reject `.feature` file syntax (`Feature:`, `Scenario:`, `Scenario Outline:`, `Examples:`, `Background:`) — the Gherkin-style convention explicitly does not use these
4. If any testable item lacks the Given/When/Then pattern → FAIL with item ID and quoted description

**PASS:** All testable items use Gherkin-style description format

**FAIL:** One or more testable items have non-compliant description format (actionable)

**Check method:** Acceptance item description × verification_type — format pattern check

---

### Sub-check 5e — Falsifiability (NEW)

**Purpose:** Every acceptance item's `pass_condition` must be falsifiable — there must exist a concrete, identifiable scenario where the implementation could fail it. An unfalsifiable pass_condition can be "satisfied" by any implementation, making the item meaningless as a quality gate.

**Execution steps:**

For each acceptance item:

1. Read `pass_condition`
2. Apply first-principles reasoning: "If the implementation were broken, would there be a way for this pass_condition to reveal it?"
3. Identify a specific failure scenario that would cause the pass_condition to FAIL:
   - Concrete: names a specific behavior change ("returns 200 instead of 201", "missing field X in response", "allows duplicate email registration")
   - Observable: the failure could be detected by reading code or test output
   - Distinct: the failure scenario is different from "the code doesn't exist" (that's structural, covered by Step 1)
4. If agent cannot identify a concrete failure scenario → the item is unfalsifiable
5. Write an explicit statement for each item: "This item would FAIL if [specific code behavior or condition] occurs."

**Reports:**

```
{item.id}: PASS
  - pass_condition: "Returns HTTP 201 with {id, email, created_at}"
  - Would FAIL if: missing created_at field, returns 200, returns 500 on valid input

{item.id}: FAIL — Unfalsifiable
  - pass_condition: "registration works correctly"
  - "works correctly" is a value judgment, not a verifiable condition.
    No concrete code change would cause this specific condition to FAIL.
  - actionable: Replace with a specific, measurable pass_condition
```

**Edge cases:**
- Pass conditions referring to external systems or runtime constraints that are not statically verifiable → not automatically unfalsifiable. Agent records them as CANNOT_DETERMINE in verify but does not fail validate.

**PASS:** All items have an identifiable failure scenario

**FAIL:** One or more items are unfalsifiable

**Check method:** First-principles reasoning — agent must articulate a concrete failure mode and write an explicit "Would FAIL if" statement per item

---

### Sub-check 5f — Description actionability (NEW)

**Purpose:** For `verification_type: testable` items, the `description` must contain enough detail to derive specific test scenarios. A vague description produces vague tests — or leaves the agent to invent scenarios that don't reflect the author's intent.

**Execution steps:**

1. For each acceptance item with `verification_type: testable`:
   - Read `description`
   - Judge whether it contains enough information to derive:
     - Specific input values or conditions
     - Expected output or state change
     - At least one boundary or edge case
2. If the description is a single vague sentence with no scenario breakdown → FAIL
3. If the description is short but specific (e.g., "Returns 201 when valid email and password are provided") → PASS
4. If the description is long but purely narrative with no testable specifics → FAIL

**Reports:**

```
{item.id}: PASS
  - description: "User registers with email and password. Returns 201 with {id, email, created_at}. Returns 409 if email exists."
  - Verdict: Enough detail to derive happy path, conflict scenario

{item.id}: FAIL — Description too vague for test derivation
  - description: "User can register"
  - Verdict: No inputs, no expected outputs, no conditions.
  - actionable: Expand description with Given/When/Then scenarios or specific pass conditions
```

**PASS:** All testable items have actionable descriptions

**FAIL:** One or more testable items lack sufficient descriptive detail for test derivation

**Check method:** Semantic assessment — agent judges whether description is specific enough to derive test inputs and expected outcomes

---

### Sub-check 5g — Pass condition / description coupling (NEW)

**Purpose:** The `pass_condition` must add specific, verifiable constraints beyond what the `description` already communicates. If the pass_condition merely rephrases the description in vaguer or equivalent terms, it contributes no value and the acceptance item cannot be meaningfully verified.

**Execution steps:**

For each acceptance item:

1. Read both `description` and `pass_condition`
2. Compare them semantically:
   - Does `pass_condition` reference specific values, status codes, field names, error types, state transitions, timeouts, or behavior variants that the `description` does not?
   - Is the `pass_condition` more abstract or vague than the `description`?
3. If the `pass_condition` is semantically equivalent to or vaguer than the `description` → FAIL

**Examples:**

```
FAIL — pass_condition adds nothing:
  description:  "User registers with email and password"
  pass_condition: "registration completes successfully"
  → "completes successfully" is a vague rephrase of "registers"

PASS — pass_condition adds specific constraints:
  description:  "User registers with email and password"
  pass_condition: "Returns 201 with {id, email, created_at}. Returns 409 if email exists. Email field must be normalized to lowercase."
  → Adds three verifiable constraints not present in description

PASS — pass_condition provides complementary information:
  description:  "System exports data as CSV"
  pass_condition: "File is valid CSV: comma-separated, quoted strings, header row matches schema fields"
  → Adds specific format criteria not in description
```

**PASS:** All pass_conditions add specific value beyond their descriptions

**FAIL:** One or more pass_conditions are vague rephrasings of their descriptions

**Check method:** Semantic cross-reference — agent compares information content of description vs pass_condition

---

### Sub-check 5h — Contract statement carry-over (NEW)

**Purpose:** Contract statements in the spec body and non-evidence appendices must be carried by a formal behavior carrier (the acceptance item set or a protocol appendix, see `framework/spec_writing_guide.md` §4). The cross-unit check reads only carriers, so a contract that lives only in prose is invisible to every dependent unit.

**Execution steps:**

1. Extract **contract statements** from the spec body and every non-evidence, non-exempt appendix:
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

**PASS:** Every contract statement in prose is carried by an item or protocol appendix

**FAIL:** One or more contract statements lack carrier coverage (actionable)

**Check method:** Body + non-evidence appendices × (acceptance item set ∪ protocol appendices) — contract-level cross-reference, quoted evidence per statement

---

### Sub-check 5i — Contract substance (NEW)

**Purpose:** The acceptance item set is the primary formal behavior carrier; empty-but-compliant items ("correct handling", "behaves as expected") leave the cross-unit check nothing to compare against. Every item must satisfy the Contract Substance Baseline, enforced here.

**Execution steps:**

For each acceptance item, apply the five baseline rules (S1–S5, see `framework/spec_writing_guide.md` §7 Contract Substance Baseline):

1. **S1 — Contract element sufficiency:** `description` and/or `pass_condition` must carry at least one concrete contract element (numeric constraint, HTTP status code, field/type/enum name, error code, protocol format, timing/consistency assumption). Pure behavior narration with no element → FAIL (e.g. "User can log in", "System handles requests correctly")
2. **S2 — Specific-value obligation:** constraints use concrete values — `201` not `2xx`, `5s` not "fast", methods enumerated (`OAuth2 + API Key`) not "multiple methods". Generalized values → FAIL
3. **S3 — Scenario completeness:** the Gherkin scenario set includes the happy path plus at least one failure or boundary scenario. Happy-path-only items with a testable verification type → FAIL (non-testable items: the pass_condition must carry at least one failure/edge condition instead)
4. **S4 — Information increment:** `pass_condition` carries constraints the `description` does not. Pure rephrase → FAIL (overlaps Check 5g by design — 5g judges value, 5i judges substance; both are FAIL-level)
5. **S5 — No template phrasing:** no content-free boilerplate ("behaves as expected", "processed correctly", "meets user expectations") → FAIL

**Relationship to 5e/5f/5g:** 5e judges falsifiability, 5f actionability, 5g information increment — 5i judges **contract information content** (presence of concrete elements, specific values, scenario coverage, boilerplate). The checks are complementary; a single item may fail several at once. When an item fails 5i, quote the violated rule and the offending text.

**PASS:** All items satisfy the Contract Substance Baseline

**FAIL:** One or more items violate S1–S5 (actionable, with the violated rule quoted)

**Check method:** Contract Substance Baseline × acceptance item set — rule-by-rule semantic assessment with quoted evidence

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
  - An appendix with `status: retired` must not be referenced: the retiring appendix is removed on promote, so the reference would break — FAIL (actionable: remove the reference or remove the `status: retired` declaration). The mechanical `specflowctl validate` Check 4 and `specflowctl promote` reject the same reference (a retiring spec's own references are exempt).
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

**PASS:** All affects declarations are valid, evidence appendix is semantically consistent, appendix file path references exist

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
7. **Carrier substance warning:** if a dependency unit's carriers (item set or protocol appendices) are compliant but carry no comparable contract statements (e.g. items with no concrete values, codes, or formats to compare against) → WARNING naming the unit and the empty carriers, recommending the dependency unit enrich its acceptance items per `framework/spec_writing_guide.md` §7 Contract Substance Baseline. Not a FAIL — the dependency unit's own validate Check 5i gates empty carriers at promote; this warning surfaces the coupling risk to the user.

**Dependency scope report:** Report the carrier regions actually depended on — the dependency unit's acceptance item set and the whole protocol appendix files. The item set is declared as a **structural region dependency** (`specflowctl gate-evidence --file <path> --acceptance-items`, see `framework/validation_cache.md` §Structural Region Dependencies): it is located by structure, so prose edits elsewhere in the same file — even inside the same content-defined chunk — do not stale this cache. When the no-contradiction assertion also reads a dependency unit's contract section (e.g. a protocol description in a named `##` section), declare that section region too (`--section <heading>`). Protocol appendices are contract files and are declared whole. Cache entries for dependency unit main specs, their protocol appendices, and rule files are written as **logical references** (`unit:{name}`, `unit:{name}:appendix:{file}`, `rule:{id}`) instead of physical paths — freshness resolves them to the current-layer file (candidate first, stable fallback), so promoting the referenced unit or rule does not stale a cache whose dependency content is unchanged (see `framework/validation_cache.md` §Logical References).

**PASS:** No contradictions across related units; acknowledged contract changes are declared

**WARNING (step 7):** Dependency unit carriers carry no comparable contract statements — consider enriching them per the Contract Substance Baseline

**FAIL:** Contradiction found (actionable) or unacknowledged contract breakage (needs_decision)

**Check method:** Candidate × related candidates × related stables three-way cross-reference over the formal behavior carriers (acceptance item sets + protocol appendices).

---

## Check 8 — Constraint alignment

**Purpose:** The candidate design must not violate global constraints or bound rules.

**Execution steps:**

1. Read the stable global rule set (`docs/specs/rules/stable/g_rule_*.md`) and each bound rule listed in `rule_refs`. Stable global rules apply to every current-layer unit by default and are not repeated in `rule_refs` (see `framework/spec_writing_guide.md` §5). Execute the circular-dependency and layer-order prohibitions exactly as recorded in `g_rule_repository_baseline.md` §6.1 items 4-5 — the rule file carries the graph derivation, tooling cross-check, fail-closed behavior, and resolution path. Rule file reads are declared as logical references (`rule:{id}`) in the validate cache (see `framework/validation_cache.md` §Logical References).
2. Check the candidate design against each global rule and each bound rule:
```
   - Is every "must not" prohibition respected?
   - Is every "must" requirement satisfied?
```
3. **Cycle resolution guidance (when the dependency graph contains a cycle):** follow the resolution path recorded in `g_rule_repository_baseline.md` §6.1 item 4 — analyze the cycle's nature (extract the shared contract region into a rule so the dependencies become star-shaped, or re-draw the unit boundaries), present the analysis to the user, and apply the fix only after explicit approval.
4. **Rule exception re-evaluation:** Read the candidate spec's frontmatter `rule_exceptions` field (see `framework/spec_writing_guide.md` §3). For every recorded exception, first verify its reference validity, then re-evaluate whether the exception still holds against the current implementation and the current rule version:
   - Referenced rule is neither a stable global rule nor a bound rule listed in this unit's `rule_refs`, or the reason is missing → FAIL (actionable: correct or remove the invalid exception entry)
   - Exception no longer justified (architecture was rewritten, rule changed, or the reason expired) → FAIL (actionable: report the exception for removal; the removal is applied only after user approval)
   - Exception still justified → keep it and state the re-examination verdict in this check's reason
5. **Appendix constraint check:** Include appendix design descriptions, API contracts, and behavior definitions in the constraint and bound rule checking. If appendix content describes behavior that violates a global constraint or bound rule → FAIL (actionable: align appendix content with constraints)

**PASS:** Candidate (main spec + appendices) is compatible with all constraints and bound rules; all recorded rule exceptions are still justified

**FAIL:** Constraint or rule violation found in main spec or appendices (actionable)

**Check method:** Candidate × stable global rule set × bound rules three-way cross-reference

---

## Step 9 — Write validate cache (main agent)

After all 8 checks complete:

- **If all PASS:** write validate cache per `framework/validation_cache.md` format:
  - Create `docs/specs/meta/validation/unit/{name}/` directory if needed
  - Collect dependency evidence for every file read during validation, including:
    - Main spec file
    - Every non-exempt appendix file
    - All referenced files (unit_refs, rule_refs, affects.files are already included)
  - For each file, run `specflowctl gate-evidence --file <path> --ranges <lines>` and record its `hash` + `deps` output in the cache's `files` entry. The `--ranges` values come from the sub-agent's `Dependency scope` report (see §Output Format); a file reported as `all` (or not reported) is declared without `--ranges` (whole file). The declared ranges must cover every region the validation judgment depended on — when unsure, declare more (declare-heavy principle; see `framework/validation_cache.md` §Dependency Declaration). **The unit's own main spec is declared by section regions, not line ranges:** run `specflowctl gate-evidence --file <spec> --section <heading>...` per check (the `--section` values come from the sub-agent's per-check `Dependency scope` lines — the section-region headings the check declared) and record the per-check breakdown in the entry's `checks` mapping with the union in `deps` (see `framework/validation_cache.md` §Format → Per-check evidence). **Union discipline: the file-level `deps` must include every per-check dep** — the promote gate fails closed when a check dep is missing from the union (false-fresh protection), and extra union deps beyond the check set are legal (declare-heavy). **Cross-unit and rule dependency entries are written as logical references** — `unit:{name}` for a dependency unit main spec read by Check 7, `unit:{name}:appendix:{file}` for a dependency unit protocol appendix read by Check 7 (the full appendix file base name without `.md`, e.g. `unit:auth:appendix:unit_auth_account_token_claims`), `rule:{id}` for a rule file read by Check 8 — with the `hash` + `deps` of the file the sub-agent actually read (see §Logical References)
  - Write `validate_result.md` with `result: pass`, `target: candidate`, `mode: full`, file hashes and dependency CIDs
  - Targeted runs (`:check-{n}` / `:{keyword}`) never write a cache, and a targeted run that FAILs deletes the existing cache — any FAIL at any granularity means promote must not proceed — see `framework/validation_cache.md`

- **If any FAIL:** delete existing `validate_result.md` if present. Do not write cache. Proceed to Present Findings.

### Delta re-run (revalidate@{unit})

Candidate targets, plus stable-only targets with a usable confirmation baseline — a delta re-run against a stable-only target whose confirmation cache is MISSING or BLOCKED reports "No usable confirmation baseline. Run the full command" and stops (see `framework/verification_scope.md` §Delta Runs → Layer applicability). Triggered by the user when the cache is STALE. Follow §Delta Runs in `framework/verification_scope.md`:

1. **Preconditions** — the existing cache must have `mode: full` and `result: pass`; a MISSING cache or a blocked review cache (for rereview) has no usable baseline — run the full command instead.
2. **Scope derivation (mechanism-derived)** — run `fresh@{unit}` and read its `DELTA SCOPE (validate)` section: the affected check keys (from the cache's per-check `checks` mapping), the unclaimed entries, the stale-dep count, and the degradation state. Affected checks = {checks whose declared deps went stale} ∪ {cross-check}. Stale deps that no check declared are **unclaimed** — map them by the fixed associations: a `unit:{name}` / `unit:{name}:appendix:{file}` entry → Check 7 (cross-unit); a `rule:{id}` entry → Check 8 (constraint alignment); a whole-file or declare-heavy stale dep in the unit's own file → re-read the spec and map it semantically. A cache without per-check evidence (legacy — `fresh@` reports "no per-check evidence") falls back to semantic derivation: map the changed content to the affected checks by re-reading the spec. If `fresh@{unit}` reports STALE for an entry the derivation sees as unaffected, trust `fresh@` — treat the file as a stale source and map its unclaimed deps per the rules above (see `framework/verification_scope.md` §Delta Runs). Report the scope — re-run checks with their stale source and reason, and the carried-over checks — before executing.
3. **Degradation (derived)** — when the affected set covers every declared check, the incremental scope is the whole run: report this to the user ("the stale sources affect every check — an incremental re-run is a full re-run") and ask whether to proceed or run `validate@{unit}` instead. Editing one section of your own spec stales only the checks that declared it — this is NOT a degradation trigger.
4. **Execution** — re-run only the scoped checks + cross-check. Any P0/P1 finding → delete the validate cache, stop, present findings (same as full FAIL).
5. **On PASS** — rewrite `validate_result.md` with `mode: full`, `basis: delta`, `target: candidate` (stable-only target: `target: stable`), a fresh `timestamp`, and a **complete** `files` list: new `hash` + `deps` + per-check `checks` evidence (`specflowctl gate-evidence`) for the re-run checks' files, the original evidence for the carried-over checks' files (their CIDs are unchanged by construction — they were not stale sources). Logical references (`unit:{name}` / `unit:{name}:appendix:{file}` / `rule:{id}`) are carried over the same way.

---

## Present Findings

Advisory findings from Check 2 Step 4 are presented for awareness only — they enter neither the batch group nor the decision group, need no decision, and do not block the flow. They are presented on their check line's reason even when all checks PASS. Each advisory finding carries its severity confirmation record (`confirmed` / `adjusted: {Px} → {Py}` + evidence) from Check 2 Step 4.

### Batch classification (validate)

When FAIL items exist, the main agent classifies each finding into a **batch group** or a **decision group** before presenting. Classification uses each FAIL's resolution type (actionable / needs_decision) and the check definition — it adds no new analysis; for batch group candidates it only runs lightweight assertion re-verification (see Assertion re-verification below).

**Batch group eligibility:** A finding enters the batch group only if its fix is fully determined by an objective standard — no interpretation of design intent is required. The batch group is limited to these fix types:
- Check 1: missing required frontmatter fields (standard: the required fields list)
- Check 1: unit_refs / rule_refs / appendix references to non-existent files (standard: file existence)
- Check 1: appendix path or naming not following the convention (standard: the path convention)
- Check 5d: testable item description missing Given/When/Then (standard: the Gherkin-style convention in `framework/spec_writing_guide.md`)

All other FAIL findings — including 5e/5f/5g content rewrites, Checks 2/3/4/6/7/8, and every needs_decision item — go to the decision group. **needs_decision items always go to the decision group.**

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

### Severity check

validate grades findings P0/P1. P1 is the contract-decided default for every FAIL check — it needs no confirmation. A P0 grade is a judgment-based assignment and must be confirmed per `framework/severity_policy.md` §9: read the impact surface the P0 claim depends on (the downstream consumer, dependent unit, or the section/appendix the claim relies on), verify the §9.3 boundary holds, and record `confirmed` or `adjusted: P0 → P1` with the evidence file and reason. Check 2 Step 4 advisory findings are confirmed under the same rules (see Check 2 Step 4). The records appear in the report so the trace shows the check ran.

```
Severity check:
  confirmed: N | adjusted: N
  - {location} — adjusted {Px} → {Py}, evidence: {file}, reason: {one line}
```

### Summary format

Present the findings using the unified report skeleton (§Output Format). The header, `Blocking promote`, key counts, and `Next step` follow the skeleton; the Severity check and Findings sections are command-specific and defined here.

```
────────────────────────────────────────────
validate@{unit} · full · candidate   # targeted runs: validate@{unit} · targeted (user requested: {keyword}) · candidate
Result: FAIL
Blocking promote: yes
Key counts: Findings: N (P0: a | P1: b | P2: c | P3: d)
────────────────────────────────────────────
{body — the executed check lines, one per check; Failed checks: N and Advisory findings: K shown in the body}
────────────────────────────────────────────
Severity check:
  confirmed: N | adjusted: N
  - {location} — adjusted {Px} → {Py}, evidence: {file}, reason: {one line}
────────────────────────────────────────────
Findings:
  Batch group (N items) — fix fully determined by an objective standard:
    - [{severity}] {location} — {issue} (actionable, based on: {standard reference})
    ...
  Decision group (M items) — need confirmation:
    1. [{severity}] {location} — {issue} (actionable | needs_decision)
    ...
────────────────────────────────────────────
Next step: {actionable → "fixes applied — re-run `validate@{unit}:check-{n}` to confirm"; needs_decision → "awaiting your decision on {item}"}
────────────────────────────────────────────
```

`Findings` (N) equals the sum of batch group items and decision group items.

When no finding qualifies for the batch group, present flat:

```
Findings:
  1. [{severity}] {location} — {issue} (actionable | needs_decision)
  ...
```

### Validate-specific notes

- **Re-validation rule:** After any fix is applied, the agent must NOT re-run validate automatically. Executing quality-gate commands is user-triggered only (see HARD RULE 2 in `framework/concepts.md`). The agent guides the user to a targeted re-check with the concrete command and waits for the user to trigger it. Affected-check mapping: acceptance item edits (any field) affect the Check 5 family — suggest `validate@{unit}:check-5`, since every sub-check reads item fields; `affects.*` edits additionally affect Check 6 — suggest `validate@{unit}:check-5` plus `validate@{unit}:check-6`; edits to spec body prose or appendices affect 5a/5b — suggest `validate@{unit}:check-5`. Example suggestion: "Fixes applied. Suggest re-running `validate@{unit}:check-{n}` to confirm. Shall I run it?" (Targeted re-checks never write a cache — only a user-triggered `validate@{unit}` full run restores the cache.) Until a re-run is triggered, do NOT write a pass cache and do NOT claim the fix is verified — report "fixed, pending re-confirmation". When a re-run is triggered by the user, the final validate result is based on the re-run, not on the pre-fix snapshot. Findings from the pre-fix snapshot that are no longer reproducible on the re-run are dropped, not carried forward as still-open. When a re-run changes an earlier finding, inform the user: "Re-validated affected checks after the fix. [finding] no longer holds. Remaining findings: ..."
- **needs_decision items** (resolution_type: needs_decision) require user input — skip to next finding without suggesting a fix.

# Validate Checklist

## Overview

When an agent executes `validate@{unit}`, it uses the 10 checks defined here. The trigger route in `framework/concepts.md` loads this file at validate time, not proactively.

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
| `validate@{unit}` | full | All 10 checks + cross-check. Quality checks are holistic — always runs full. |
| `validate@{unit}:check-{n}` | targeted | Single check `{n}` only. User explicitly chooses focus. Does not write a cache. |
| `validate@{unit}:{keyword}` | targeted | Match keyword to check name (e.g., "design" → Check 2, "scope" → Check 3). User explicitly chooses focus. Does not write a cache. |

**Keyword domain:** validate keywords resolve to check names — `structure` (Check 1), `design` (Check 2), `scope` (Check 3), `evidence` (Check 4), `acceptance`/`coverage` (Check 5), `affects` (Check 6), `cross-unit` (Check 7), `constraint` (Check 8), `ownership`/`surface` (Check 9), `reader`/`readable` (Check 10). A keyword matching no check name is a no-match — ask the user for clarification.

**Output:** Targeted runs report only the executed check(s) and note "This was a targeted check — no complete cache was written. Run `validate@{unit}` for a complete validation." A targeted P0/P1 must first be persisted with `gate-invalidate --check {check key}`.

### Stable-only mode

When no candidate spec exists (validate against stable), run the same 10 checks + cross-check against the **stable** content:

1. Read the stable main spec: `docs/specs/units/stable/unit_{unit}.md`
2. Glob all stable appendix files: `docs/specs/units/stable/appendix/unit_{unit}_*.md`, read every non-exempt, non-retired appendix (same skip rules as the candidate path)
3. Run all 10 checks + cross-check against the stable content — Checks 6/7/8/9 are the live part: referenced files, dependency-unit contracts, rules, and declared surfaces may have changed since promote, so the stable content may no longer hold (e.g. a new rule now prohibits something the stable design does, or another unit now declares a file this surface also declares)
4. **PASS** → `gate-finalize` writes the validate cache with `target: stable` (confirmation state consumed by `fresh@stable`; `mode: full`, `hash` + `deps` evidence; same packet sequence as Step 9)
5. **FAIL** → `gate-finalize` writes a failure record (`result: fail` + `blocking: true`, `mode: full`, `basis: full`, and the per-check `status` map — `pass`/`fail` for every executed check plus the cross-check; full runs have no `carried` — the confirmation state stays visible as BLOCKED and is the failure-recovery baseline), present the findings (Check 2 Step 1 and 5a/5h FAIL findings re-verified per §Step 9 → Extraction re-verification before presentation), and recommend forking the unit (`specflowctl fork --unit <name>`) to reconcile the stable content with the changed dependency or rule. Do not edit stable directly; normal candidate-to-stable writes use promote (the routed remove/update procedures own their narrow exceptions)

The stable confirmation cache is read-only state: it grants no promote eligibility (stable has no gate). Delta re-runs (`revalidate`) apply to stable-only targets with a usable baseline — a pass cache (`result: pass`, STALE recovery) or a failure record (failure-record recovery); a MISSING stable cache needs the full confirmation run — see `framework/verification_scope.md` §Stable-only Targets and §Delta Runs → Layer applicability.

## Execution Rules

- **Subagent permissions:** validate executes one packet per independent read-only sub-agent session — the sub-agent may inspect file content, search text by pattern, and locate files by name pattern. Must NOT modify files, execute commands, or delegate to other agents. The main agent sends `specflowctl gate-packet --format prompt` output verbatim; its check keys are the packet scope (see `framework/verification_scope.md` §Sub-agent Prompt Assembly). Targeted runs (`:check-{n}` / `:{keyword}`) execute directly in the main agent session instead — Check 10 is the exception: its targeted run coordinates the reader and verifier sessions per Check 10, still without a packet run or cache write.
- Each check reports **PASS**, **WARNING**, or **FAIL** with a reason.
- On FAIL, the agent must identify **which information sources contradict each other** (e.g., "spec body describes auto-retry logic but no acceptance item covers it") in the FAIL reason, and record the fix in the FAIL reason. When the finding is written out, its `evidence:` block quotes the contradicting sources verbatim and its `fix:` (or `decision:`) field states the repair — the entry line's location reference is a pointer, not the evidence.
- Resolution types:
  - **actionable** — A concrete repair can be made inside the current candidate spec without user judgment.
  - **needs_decision** — Requires user input (unclear intent, missing decision, or external dependency). Stop and ask.

## Output Format

==ATOM_BEGIN:report_skeleton==
## Unified Report Skeleton

All quality-gate reports (validate, verify, review) share the same report skeleton below. The header lines (`Result`, `Blocking promote`, `Key counts`), the Findings block fields, and the `Next step` line are identical across commands; only the body content and the command-specific extra lines inside each finding block are command-specific and defined in each checklist file. The Findings section follows the header because on FAIL it is the decision surface — the body, Dependency scope, and Severity check sections are its audit and evidence.

```
────────────────────────────────────────────
{command}@{target} · {mode} · {layer}
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
    {command-specific detail lines}
    ref: {anchor or line reference}                # optional, tracking only
────────────────────────────────────────────
{body}
────────────────────────────────────────────
Dependency scope:
  {check key}: {file}: {declaration}   # per executed check; declaration = section heading text, acceptance item id list (acceptance_item:<id>), the whole-set token acceptance_items, line ranges, or "all"
────────────────────────────────────────────
Severity check:
  confirmed: N | adjusted: N
  Severity confirmation: {finding_id} = confirmed {Px} — evidence: {file}; reason: {one line}
  Severity confirmation: {finding_id} = adjusted {Px} -> {Py} — evidence: {file}; reason: {one line}
  Severity confirmation: {finding_id} = confirmed {Py} — evidence: {file}; reason: {required final second check after an adjustment}
────────────────────────────────────────────
Next step: {concrete next command with reason, or "None"}
────────────────────────────────────────────
```

**Field definitions:**

- `{command}@{target}` — the command and target that produced this report, e.g. `validate@user_auth`, `verify@user_auth`, `review@user_auth`. Commands: `validate`, `verify`, `review`. Targets: unit or rule name.
- `{mode}` — `full` for full runs; `targeted (user requested: {keyword})` for targeted runs; `delta` for incremental re-runs (`revalidate@{target}` / `reverify@{unit}` / `rereview@{unit}`).
- `{layer}` — the spec layer checked: `candidate` | `stable`.
- `Result` — `PASS | FAIL` for all three commands. For packet runs this value is generated by `gate-finalize` from the accepted synthesis result (or from the single rule-validate packet, which has no cross packet); the coordinator never supplies it. The gate is decided by severity: FAIL means gate-driving retained P0/P1 findings exist. validate grades findings P0/P1 only, so any validate FAIL is a P0/P1 finding; verify/review FAIL means gate-driving retained P0/P1 mismatches/findings exist. A review finding deferred to another unit is retained and routed but is not gate-driving — it never causes FAIL.
- `Blocking promote: yes | no` — `yes` when the run FAILs (gate-driving P0/P1 findings exist); `no` otherwise. Valid for all three commands.
- `Key counts: Findings: N (P0: a | P1: b | P2: c | P3: d)` — N is the gate-driving retained finding count and a/b/c/d are generated by `gate-finalize`; the coordinator never supplies them. validate grades findings P0/P1 only, so its P2/P3 counts are always 0; verify's blocking mismatches equal the P0/P1 counts and non-blocking mismatches equal the P2/P3 counts. A review finding deferred to another unit is excluded from these counts (it is presented under `Deferred to {unit}:`). Command-specific summary numbers (validate's failed checks and advisory findings, verify's coverage, review's suppressed-by-spec count) appear in the body.
- `{body}` — command-specific content defined in this file's body format section (validate: one line per check; verify: Items / Scope / Integrity / Coverage / first-principles divergence analysis; review: Architecture assessment and suppressed findings).
- `Findings:` — `Findings: none` when no finding is retained; otherwise one entry per finding: a unified entry line `[{severity}] {location} — {issue} (actionable | needs_decision)` followed by the finding block's required detail lines. `actionable` — a concrete repair can be made without user judgment (verify: direction spec_gap/code_gap; review: a determined recommendation); `needs_decision` — requires user input or a design decision before the fix can be made (verify: direction needs_design/blocked; review: architecture trade-offs). The block is written for a reader who has not opened the spec or the code: delete every `ref:` line and every `§`, line-number, or other anchor reference from the field text, and the reader must still learn what is wrong, what the conflicting sides say, what is at risk, and what to fix or decide. Anchors appear only in the optional trailing `ref:` line, which carries tracking information and no meaning.
  - `problem:` — one sentence naming the concrete subject (acceptance item id, field or parameter name, behavior) and what is wrong with it; never an anchor, region name, or pointer.
  - `evidence:` — the quoted content that proves the claim, as one or more indented `- {verbatim quoted source content} — {source label}` sub-lines. Contradiction findings quote both conflicting sides verbatim; undefined or missing behavior lists the defined part and the missing part; spec-vs-code findings quote the declared behavior and the implemented behavior. A review P3 finding uses its `fact_anchor:` line as this evidence form.
  - `impact:` — what goes wrong, or stays undecidable, if the finding is not resolved.
  - `fix:` — actionable findings: the concrete repair action. `decision:` — needs_decision findings: the question the user must answer, with `options:` sub-lines listing the candidate choices.
  - The block's detail lines are contiguous indented lines directly under the entry line — no blank lines inside the block (the block is stored and re-rendered verbatim when a finding is carried into a delta/repair run).
  - `{command-specific detail lines}` — command-specific extras follow the shared fields: verify's `root_cause:`, `direction:`, and `confidence:`; review's `spec_context:` and its mechanically required `fact_anchor:` for P3 findings. validate and rule validate add none.
  - Findings are grouped into the batch group and decision group defined in this file's batch classification section when this file defines one; flat when this file defines no batch classification or grouping is inactive. Batch-group entries carry the same fields; terse one-line values are fine.
  - Review reports present findings whose recorded ownership belongs to another unit in a `Deferred to {unit}:` section after the Findings section, with the same block fields plus an `ownership:` line. They are retained and routed but do not count toward `Key counts`, `Blocking promote`, or the gate result (see `framework/spec_review_checklist.md` §Output Format → Deferred findings).
- `Dependency scope:` — one line per check the run executed: `{check key}: {file}: {declaration}`. `{check key}` is the command's check identifier (validate: `check-{n}`; verify: the acceptance item id; review: the packet's file path). `{declaration}` is the section-region heading text the check's judgment read (e.g. `Description`, `Testability / Acceptance Criteria`; the frontmatter region is `frontmatter`), `acceptance_item:<id>[,<id>...]` (one or more acceptance item regions of the spec's `acceptance_item_set` — the precise declaration for a judgment over specific items, e.g. one verify item judgment), the reserved token `acceptance_items` (the whole `acceptance_item_set` structural region — for judgments over the set as a whole, e.g. validate's acceptance coverage check), 1-based closed line ranges (e.g. `120-180,300-320`), or `all` when the judgment covered the whole file. Every read-only subagent reports this scope for the checks in its packet; the report is submitted verbatim via `gate-submit`, which validates the declarations against that packet's `read_refs` — not merely the run-wide snapshot — (path membership and declaration parseability), and `gate-finalize` computes the CIDs and records the per-check breakdown in the cache's `checks` mapping (see `framework/validation_cache.md` §Format → Per-check evidence). Delta/repair runs report the scope of the re-run checks only — carried-over checks are not re-executed and get no new declaration (see `framework/verification_scope.md` §Sub-agent Prompt Assembly Check / Packet scope). Targeted runs may omit it.
- `Severity check:` — packet-run reports use the exact `Severity confirmation:` line grammar owned by `framework/verification_scope.md` §Gate Work Packets → Packet report contract. Unit cross reports carry one complete sequence for every terminal retained finding after disposition/merge resolution: one `confirmed` record, or an `adjusted` record followed by exactly one final record for the adjusted severity. `confirmed: N | adjusted: N` counts findings by final sequence outcome, not record lines. Rule validate reports carry the confirmation sequences required by their command checklist. `gate-submit` parses and validates these records; `gate-finalize` uses the resulting canonical severities. Targeted reports retain the command checklist's human-readable severity trace but do not create packet state.
- `Incremental scope:` — delta runs only (mode `delta`). One line per re-run check in the run's own structure (e.g. validate: "check-5 (acceptance coverage & correctness): re-run — section `Description` of the unit's own spec changed"), followed by a line declaring the carried-over checks ("checks 1-4, 6-10: carried over — their dependency evidence is unchanged") and the cross-check result (unit targets — rules have no cross-check). The scope is mechanism-derived at plan time from the cache's per-check evidence: `gate-plan` maps stale regions to the checks that declared them, adds the current keys the baseline never declared (a new acceptance item or review file has no evidence to carry, so it executes like a stale judgment), and generates the re-run packet set (see `framework/verification_scope.md` §Delta Runs). For a failure-record recovery (`basis: repair` — the run recovers a delta FAIL's failure record or a full-run FAIL's record), the plan's packet set is the record's failed checks plus its persisted `invalidated_checks` (written by `gate-invalidate` after a targeted P0/P1), the newly affected checks, the current keys the baseline never declared, any explicit `--rerun` overrides, and the cross-check; carried-over checks are the remaining `pass`/`carried` entries (see `framework/verification_scope.md` §Delta Runs → Failure recovery). A failure record whose per-check status map is absent or incomplete (legacy or malformed), or whose invalidated key cannot map to the current judgment surface, degrades the plan to the full packet set — nothing is carried over. When the re-run covers every declared check, the plan covers the full scope — nothing is carried over. The incremental scope is reported by `gate-plan` before execution begins (the user must see what will be re-run and what will be carried over) and again in the final report.
- `Next step:` — the concrete command to run next with its reason; `None` when nothing further is needed. A finding's fix lifecycle has three states with fixed wording: `finding_open` → "Resolve the findings, then re-run the target-appropriate re-check command (`validate@{target}:check-{n}`; unit targets also `verify@{target}:{keyword}` / `review@{target}:{keyword}`) to confirm"; `fixed_pending_recheck` → "Fixes applied; re-run the target-appropriate re-check command to confirm." — only after the approved fix was actually written; `verified` → "Re-check passed." — only after a re-check confirmed the fix. A gate report is always produced before any fix is applied (nothing is implemented before the user approves the findings), so an actionable finding's report-time `Next step` is always the `finding_open` wording. Other guidance: all gates green → "if the design is finalized, run `promote@{target}`"; needs_decision → "awaiting your decision on {item}"; nothing further → `None`.

**Targeted runs:** end the report with the command's targeted note ("This was a targeted check — no complete cache was written. Run `{command}@{target}` for a complete ...") after the `Next step` line. If the result contains P0/P1, run `gate-invalidate` before reporting completion.

### Completion — Persist Gate Cache

A quality-gate run is complete only when its result is persisted and visible to `fresh`/`promote`. A report alone does not satisfy the gate.

- **Full (`validate@{target}` / `verify@{unit}` / `review@{unit}`, `mode: full`, `basis: full`)** — complete only when the packet run finished and its cache was written: (1) `specflowctl gate-plan --gate {validate|verify|review} (--unit {name} | --rule {id}) --target {candidate|stable} [--input PATH_OR_REF]...` ran **before any executor read input** (before prompt assembly / sub-agent launch) — `--input` adds evidence that every packet may read but never creates a work packet; (2) every required detection/check/review/analysis report was submitted via `specflowctl gate-submit --run <run_id> --packet <packet_id> --report PATH` and accepted (verify analysis packets are `not_required` when their detection packet is not `MISMATCH`); (3) the cross packet consumed the accepted packet results and was accepted (unit targets only; rule validate has one complete packet and no cross); (4) the coordinator ran `specflowctl gate-finalize --run <run_id>` and the cache `docs/specs/meta/validation/{unit|rule}/{name}/validate_result.md` | `verify_result.md` | `review_result.md` was written; (5) `fresh@{target}` (`fresh --unit {name}` / `fresh --rule {id}`) shows the expected gate state (`FRESH` for `result: pass` / `blocking: false`, `BLOCKED` for `result: fail` / `blocking: true`). A candidate **full-run** FAIL writes a failure record (`result: fail` / `blocking: true`; validate/verify records declare `pass`/`fail` for every executed judgment) — the failure-recovery baseline. A finalize that finds any input changed since `gate-plan` is rejected and writes no cache. See `framework/validation_cache.md` §Write Rules and §Failure handling by gate role.
- **Delta (`revalidate@{target}` / `reverify@{unit}` / `rereview@{unit}`, `mode: full`, `basis: delta` or `basis: repair`)** — the same packet sequence with `specflowctl gate-plan --mode delta` (or `--mode repair` from a failure record): the plan derives the re-run packet set mechanically and records the carried-over checks; `gate-finalize` merges the carried evidence from the baseline cache. See `framework/verification_scope.md` §Delta Runs.
- **Targeted (`:check-{n}` / `:{keyword}`, no complete cache)** — complete when the report is produced and, for P0/P1, the coordinator has run `specflowctl gate-invalidate` for every affected check key. Targeted runs intentionally do NOT create a gate run — `gate-plan` / `gate-submit` / `gate-finalize` are not run. A targeted PASS leaves cache state unchanged. A targeted P0/P1 deletes a pass cache or persists `invalidated_checks` on a failure record and invalidates any matching open run; it never publishes a complete result cache or satisfies the promote gate. The report ends with `This was a targeted check — no complete cache was written. Run {command}@{target} for a complete ...`.

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
  5a. Coverage & item-set correspondence: PASS | WARNING | FAIL — reason
  5b. Content alignment: PASS | FAIL — reason
  5c. Internal consistency: PASS | FAIL — reason
  5d. Description format compliance: PASS | FAIL — reason
  5e. Falsifiability: PASS | FAIL — reason
  5f. Description actionability: PASS | FAIL — reason
  5g. Pass condition / description coupling: PASS | FAIL — reason
  5h. Contract statement carry-over: PASS | FAIL — reason
  5i. Contract substance: PASS | FAIL — reason
6. Affects-source validity: PASS | FAIL — reason
7. Cross-unit consistency: PASS | WARNING | FAIL — reason
8. Constraint alignment: PASS | FAIL — reason
9. Surface ownership & sharing: PASS | FAIL — reason
10. Reader contract: PASS | FAIL — reason
```

After the check lines, report the cross-check line (full runs only) and the counts:

```
Cross-check: 3/3 PASS — per-check results; contradictions are presented as findings
Failed checks: N | Advisory findings: K
```

**Counting rules:**
- `Findings: N (P0: a | P1: b | P2: c | P3: d)` — N is the total number of distinct findings across all FAIL checks (quality-bar findings merged per the per-item merge rule, see Per-item merge rule below); a/b/c/d the count per severity. validate grades findings P0/P1 only — P1 is the contract-decided default and its required `confirmed` record states that the default stands without a §9 boundary check; a P0 grade requires the §9 boundary check (see Severity check below) — so `c` and `d` are always 0. In targeted runs, only executed checks are counted.
- `Failed checks` is the number of FAIL checks among executed checks, shown in the body's check lines. WARNING is not a failed check.
- `Advisory findings` (Check 1 step 7 hygiene WARNING, Check 2 Step 4 taste-level P2/P3, Check 3 step 6 split-candidate WARNING, Check 5a step 7 merge-candidate WARNING, Check 7 step 7 carrier-substance WARNING) are presented on their check line's reason and counted separately as `Advisory findings: K` in the body. They are never counted in `Findings` and never affect `Failed checks`.
- The same counts are reused in the Present Findings summary (`Findings` N = batch group items + decision group items).

**Multi-finding enumeration:** When a FAIL reason contains multiple distinct findings, list each finding under the check line as its own entry in the unified finding format `[{severity}] {location} — {finding} (actionable | needs_decision)`, followed by the shared finding block (§Output Format). The entry line must begin with the bracketed severity after optional indentation — the parser accepts indentation or a single leading `-`, but not a numbered prefix, so the packet report keeps entries as standalone lines; the presented summary may re-number them (`5a-1`, `5a-2`, ...) as presentation only. Each entry carries a location reference (the contradicting information sources, per Execution Rules), the finding statement, its resolution type, and the block fields:

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

**Per-item merge rule (quality-bar family):** Findings from sub-checks 5e, 5f, 5g, and 5i that reference the same acceptance item (same location) and whose fix is a rewrite of that item are merged into **one finding**. The merged entry's issue line summarizes the item defect; its `evidence:` block quotes each violated sub-check's rule and the offending text (following 5i's "quote the violated rule" format). The check lines in the body keep their individual PASS/FAIL status, but the finding entries merge everywhere: the merged entry is presented once, under the lowest-numbered violating sub-check's line (e.g. `5e-1`), and the other violating sub-checks' reasons reference it without re-enumerating — so per-check enumerations in the body and `Findings` N use the same post-merge count. The merged entry's severity is the highest among its violating sub-checks (P0 > P1); a P0 grade still requires the §9 confirmation record per the Severity check. `Findings` N counts merged findings — one item defect is one finding, regardless of how many sub-checks it violates. Findings from 5a (located at spec body regions), 5b/5c (each contradiction is an independent fix edit), and 5h (located at prose statements) are not merged — their locations and fixes differ.

When findings mix resolution types (within one check or across checks), the report presents each finding with its own resolution — a `needs_decision` finding stops the flow and requires user input per Execution Rules.

**Evidence discipline:** every check line's reason must reference the spec content the verdict covers (section title, item id, appendix name). A PASS with no reference means the check did not run — report FAIL with "no evidence basis". The check-line reference is the verdict trace; a finding separately carries the quoted content in its `evidence:` block.

---

## Check 1 — Structural integrity

**Purpose:** Verify the file is parseable and all required fields exist, as a prerequisite for all subsequent checks.

**Execution steps:**

1. Read `docs/specs/units/candidate/unit_{unit}.md` and all non-exempt appendix files (see Prerequisite)
2. Verify required frontmatter fields: `id`, `unit_refs`, `rule_refs`. The spec layer is encoded by the file path (`docs/specs/units/candidate/` vs `docs/specs/units/stable/`) — no `layer` frontmatter field is declared (see `framework/spec_writing_guide.md` §3)
3. Verify `acceptance_item_set` exists with at least one item. Each item must have: `id`, `description`, `verification_type`, `verification_surface`, `implementation_surface`, `verification_method`, `pass_condition`, `runnable`. `implementation_surface` may be the placeholder `<pending>` during design-first rounds (the path is not yet known); the placeholder must be exactly `<pending>` — variants are reported for correction. A leftover `<pending>` is a MISMATCH in verify (Step 6), so it must be replaced with the real path once the implementation exists. Every non-`<pending>` value must be a single repository-relative file or directory path resolving to at least one real file — a directory expands to its repository-content files (the files Git tracks plus untracked files that are not ignored), so semicolon lists, wildcard patterns, nonexistent paths, and directories with no repository-content files are FAIL, reported per item (the mechanical `specflowctl validate` Check 3 and verify/review planning enforce the same rule)
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
    - **The acceptance_item_set region covers every real item** — the region runs from the exact `acceptance_item_set:` marker line to the next `##` heading outside a code fence (the enclosing section's end), or through the last real line of the file. `###` and deeper headings never terminate the set: an item separated from the previous one by a `###` subheading is still part of the set and is verified. An item placed after the enclosing section's `##` heading sits outside the set and cannot be validated as an item — catch that here. Distinguish real items from fenced example blocks — a fenced `- id:` example is content, not an item (the mechanical `specflowctl validate` Check 8 covers the exact-marker and heading-format preconditions; the semantic coverage judgment is this step's)
    - **Fenced code blocks are content** — `##`-like lines inside ``` / ~~~ fences must not split regions; when a fence would visually span a section boundary, the spec needs restructuring (a fence cannot cross `##` headings — close the fence before the next heading)
    - **Every region the run will declare is locatable** — for each section the checks will declare (run `--section <heading>` probes), the heading resolves uniquely. A declaration that cannot be located fails closed at gate-finalize time; this step surfaces it at validate time
    - Fix direction: restructure the spec so the split is semantically clean (cohesion per `framework/spec_writing_guide.md` §13); do not fall back to whole-file declarations as a workaround
13. **Environment and deployment agnosticism check (FAIL):** Verify that the spec body and acceptance item set remain strictly environment-agnostic per `framework/spec_writing_guide.md` §14.1 (Environment & Temporal Agnosticism Law) and §14.2 (Anti-Pattern E):
    - Must NOT contain developer-machine absolute paths (e.g. `/Users/...`, `/home/...`, `C:\...`) in narrative text or structured fields (use project-relative paths instead).
    - Must NOT contain fixed local machine IP addresses or ports (e.g. `127.0.0.1:8080`, `localhost:3000`) as hard requirements (illustrations inside fenced code blocks must be explicitly marked as example placeholders).
    - Must NOT contain live or environment-specific credentials, tokens, or private secrets.
    - If found → FAIL with quoted text, section name, and line reference (actionable: replace with project-scoped relative paths, abstract placeholders, or configuration-driven parameters).

**PASS:** All format constraints satisfied

**WARNING (step 7):** Code file paths detected in prose sections — relocate to `implementation_surface` or `affects.files`, or convert to a concept name reference

**FAIL:** Any missing field, reference to a non-existent file, layer-prefixed spec path, or environment-specific hardcoding in prose or structured fields (actionable)

**Check method:** Unidirectional existence check (the only check that does not cross-reference, as it is the prerequisite)

**Communication note:** When suggesting Check 1 to a user, describe it as "structural integrity — verifies file structure and reference existence without evaluating design quality."

**Retiring unit:** when the candidate main spec declares `status: retired` (see `framework/spec_writing_guide.md` §8 Unit Retirement), the unit is being removed from stable. The acceptance item set is not required (agent Check 5's acceptance checks are skipped for the retired spec), and retiring appendices are skipped like exempt ones. Check 1 still verifies the required frontmatter fields. Check 10 is skipped entirely — the gate plan generates no reader or verifier packets for a retiring spec, since its human-readable boundary (the acceptance section) is not required to exist. The retiring spec's own references (`unit_refs`, `rule_refs`, appendix and evidence references) are exempt from the mechanical checks — they disappear with it. The mechanical `specflowctl validate` checks use their own numbering (Check 1 frontmatter, Check 2 acceptance items, Check 3 anchor integrity, Check 4 reference integrity, Check 5 appendix files, Check 6 layer-path check, Check 7 dependency cycle check, Check 8 region locatability, Check 9 surface ownership); of these, a retiring spec skips Check 2, 3, 4, 6, 7, 8, and 9, while Check 1 still applies and Check 5 keeps its appendix-level exempt/retired skipping. `specflowctl promote` also skips its reference checks for a retiring unit. Reference protection: a unit that is being retired must not be referenced by any other current-layer unit's `unit_refs` — the mechanical `specflowctl validate` Check 4 rejects such a reference; the agent must also report it via agent Check 7 (cross-unit) with `needs_decision` resolution until the referrer drops the reference.

---

## Check 2 — Design soundness

**Purpose:** Evaluate whether the design itself is correct and reasonable — not just whether it is well-documented — and whether the spec satisfies the decision-closure half of the `framework/spec_writing_guide.md` §9 Authoring Baseline: every implementation-affecting decision is closed, so the downstream executor is never forced to choose, and any intentionally unmade decision declares its boundary. The §9 expression points are tested separately by the reader contract probe (Check 10) through closed-book reconstruction. The subagent must actively reason about the design, not passively verify documentation completeness. Appendix content describing design decisions, API contracts, component trees, or data types is part of the unit's design and must be included in this analysis.

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
- A finding without this artifact is not presented — the independent cross packet re-verifies the artifact before classification and marks unfaithful claims suppressed (see §Step 9 → Extraction re-verification; e.g. a behavior that does serve a stated goal, or a proportionality claim that rests on preference rather than an establishable smaller surface, drops the finding).

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

Before presenting advisory findings, confirm each P2/P3 grading per `framework/severity_policy.md` §9: read the appendix or body section the finding judges and any section its impact claim depends on, beyond the section it was graded on (§9.5 Execution Rules, rule 2), verify the §9.3 boundary, and record the confirmation in the check-line trace (`confirmed` / `adjusted: {Px} → {Py}` with the evidence file). This trace is human-readable — it is not a packet `Severity confirmation:` record (those exist only for terminal retained findings in the cross packet). Advisory gradings are judgment-based and never contract-decided, so all of them are in scope.

**Step 5 — Authoring Baseline verification (decision closure)**
Verify that the spec closes every implementation-affecting decision in `framework/spec_writing_guide.md` §9: the downstream executor must not be forced to choose. The §9 expression points are NOT judged here — they are tested under reconstruction conditions by the reader contract probe (Check 10), whose independent reader must restate the design from the human-readable part alone. This step verifies the decision closure the probe does not test.

- **Input discipline:** the spec text is the ONLY input (main spec + non-exempt, non-retired appendices). Do not fill spec gaps with implementation knowledge from this session — if the spec omits a decision, the gap is real and must be reported. Reading the implementation defeats this check's purpose: a spec that only makes sense with code knowledge forces the downstream executor to choose, which is exactly what the baseline forbids.

**Decision closure (the "must close" list):** Verify that the spec closes every implementation-affecting decision: which object owns a responsibility, which entry point starts the behavior, where state or durable truth lives, how ordered steps connect, how boundary failures are reported, what the result shape means, how acceptance proves the stated responsibility.

- For each of the seven decisions: can the downstream executor determine the answer from the spec alone, without making a choice?
  - For evidence-driven acceptance items (Step 2 waiver), the closure source is the evidence appendix: verify it records the item's behavior domain as directly readable behavioral truth (per `framework/spec_writing_guide.md` §3 `evidence_appendix_ref`), not only background, motivation, or patch notes.
  - The "how acceptance proves the stated responsibility" decision is covered by Check 5's acceptance quality review — here, only confirm the acceptance items exist and can prove the stated responsibility.
- If a decision is intentionally not made, the spec must state that boundary and explain why (per `framework/spec_writing_guide.md` §9, the "If a decision is intentionally not made" rule). An open decision without a stated boundary is a FAIL.
- Granularity: verify closure, not exhaustiveness — the spec must not be inflated into an implementation manual. Coverage obligations are limited to formal behavior domains (see Check 5a Step 2 extraction premise); narrative elaboration in the body does not add coverage obligations.

**FAIL:** a Step 1 goal-means flag (default severity P1; extraction artifact required), a Step 2 rationale gap, a Step 3 critical flaw; or any of the seven decisions is left open AND not explicitly bounded with a reason (actionable: record the decision / declare the boundary; needs_decision when recording it requires user input — Execution Rules "missing decision")

**Step 6 — Abstraction level & implementation agnosticism (The Truth Ownership Check)**
Verify that the spec text adheres to `framework/spec_writing_guide.md` §14 (Truth Ownership Framework):

- **Mechanism vs. Behavior (Anti-Pattern B):** Verify the spec does not mandate internal data structures or language-level execution mechanisms (e.g. "must use sync.RWMutex", "must store records in a map[string]any", specific internal channel buffer capacities, or private struct layouts) instead of behavioral invariants and concurrency guarantees (e.g. atomicity, thread-safety, idempotent processing).
- **Causal Synchronization vs. Brittle Temporal Sleep (Anti-Pattern D):** Verify the design does not specify arbitrary physical wall-clock sleep durations (e.g. "wait 500ms and verify status") for asynchronous workflows. Asynchronous transitions must specify causal state changes ("until status is ready", "upon event reception") with bounded timeout semantics.
- **Test Double & Fixture Isolation (Anti-Pattern C):** Verify the design does not leak test doubles, mock runner names, or ephemeral test fixtures (`mockRunner`, `test_tool`) into formal architecture or component definitions.

If the spec specifies internal implementation mechanisms, arbitrary physical sleep waits, or leaks test doubles:
- **FAIL (P1: Over-specification)** (actionable: restate as observable behavioral invariants or causal state transitions)

**Step 7 — Verdict**
- PASS: goal-means aligned, per-item rationale documented (evidence-driven items waived per Step 2), all seven §9 decisions closed or explicitly bounded, no critical flaws found, and abstraction boundaries respected per §14
- FAIL: specific findings reported

**Check method:** Content reasoning + adversarial analysis + taste-level assessment + authoring baseline verification + abstraction boundary check (the subagent makes active engineering judgments)

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
6. **Unit cohesion / split-candidate detection (WARNING):** Judge whether the unit's content resolves into two or more responsibility clusters that are independently governable. A split candidate is reported as WARNING only when all three conditions hold:
   - **Distinct responsibility subjects** — the clusters' behavior subjects belong to different domains (e.g. authentication vs. notifications), not multiple scenarios of one subject.
   - **No shared design center** — no shared state, no shared actor journey, and no contract the clusters jointly define links them: a designed change in one cluster does not require a change in the other. Constraint-type content the clusters share (protocols, component contracts) is not a design center: it is extracted as a shared rule at split time (`framework/spec_writing_guide.md` §2 item 3).
   - **Independently governable** — each cluster could carry its own acceptance item set and be validated, verified, reviewed, and promoted without editing the other cluster's content (constraints shared between them would be extracted as a rule, `framework/spec_writing_guide.md` §2 item 3).
   Spanning multiple directories is not a signal by itself (`framework/spec_writing_guide.md` §2 item 2 allows cross-directory units) — the judgment reads coupling and governance lifecycle, not directory layout. This step is the unit-level mirror of sub-check 5a step 7 (over-split items are merge candidates): 5a detects one behavior domain split across items; this step detects independently governable responsibilities merged into one unit.
   A reported WARNING carries:
   - **Required evidence (no artifact, no WARNING):** quote each cluster's declared responsibility statement or behavior subjects (from the body and the acceptance item set) and state the independence judgment — which shared-state, shared-contract, and shared-journey surfaces were checked and why none links the clusters (same evidence discipline as Check 2 Step 1 and sub-check 5a step 9).
   - **Recommendation:** split the unit — one unit per responsibility (a user-confirmed structure change per `framework/spec_writing_guide.md` §2, never an automatic edit) — and extract constraints shared between the parts as rules. Do not satisfy this finding by moving content into appendices: non-exempt appendices stay inside the same unit's validation union and reduce neither its gate work set nor its governance weight.
   - **Resolution:** advisory WARNING only — it never causes FAIL, never blocks promote, is presented on the check line's reason, and is counted under `Advisory findings`. It is not a cross-synthesized finding and must never be emitted as a bracketed `[Px]` entry (validate grades findings P0/P1 only).

**PASS:** Scope is clear and self-consistent; no non-goal is violated; appendix content stays within unit scope

**WARNING (step 6):** The unit's content resolves into independently governable responsibility clusters — split candidate. Recommendation: split into one unit per responsibility (user-confirmed structure change, `framework/spec_writing_guide.md` §2); extract shared constraints as rules; non-exempt appendix offload does not reduce the unit's governance weight

**FAIL:** Ambiguous scope, goal/non-goal contradiction, boundary violation, or out-of-scope appendix content (actionable)

**Check method:** Multi-field cross-reference (goal × non-goal × behaviors × appendix content) + unit-cohesion judgment (responsibility-cluster independence)

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

**Purpose:** The spec body and acceptance items must cover each other bidirectionally — every designed behavior has an item (5a forward coverage), every design-driven item maps back to a designed behavior (5a orphan detection; evidence-driven and replacement/cleanup items excepted), over-split items are merge candidates (5a) — match semantically (5b), and contain no internal contradictions (5c). Acceptance items must also have falsifiable pass_conditions (5e), actionable descriptions (5f), and coupled pass_condition/description pairs that add value (5g).

**Execution steps:**

### Sub-check 5a — Coverage & item-set correspondence

**Purpose:** Every behavior domain in the spec body and appendices must have at least one corresponding acceptance item, every design-driven acceptance item must correspond to a designed behavior (orphan detection, step 8), and the item's surface fields must be consistent with the behavior type. Granularity baseline: behavior domains as defined in `framework/spec_writing_guide.md` §Acceptance Item Granularity — one item = one behavior domain with its full scenario set (happy path + error paths + boundary cases). Enhanced from the original forward coverage check to a bidirectional check.

**Execution steps:**

1. **Coverage input source:** A behavior is covered when any acceptance item describes it in its `description` (Given/When/Then scenarios) OR constrains it in its `pass_condition`. The coverage judgment input is the union of `description` and `pass_condition` — a behavior constraint that already appears in some item's `pass_condition` counts as covered, consistent with sub-check 5g (which requires `pass_condition` to carry constraints beyond `description`).
2. **Extraction premise (shared with sub-check 5h):** Behavior-domain extraction targets only formal behavior declared in the spec body and appendices — a behavior subject (endpoint, function, state machine, or flow entry point) together with its behavior semantics. The subject must be externally observable per `framework/spec_writing_guide.md` §4: internal implementation detail (internal field names, internal field layouts, internal timing — including retry/backoff values, internal data structures and their operations, internal function behavior, configuration layout) is design expression, not a behavior domain source, and creates no coverage obligation — the same boundary step 6 and sub-check 5h apply on their surfaces. Non-constraining narrative (design discussion, illustrative examples, motivation, variant elaboration) is NOT a behavior domain source and does not create coverage obligations. Extract all behavior domains at the granularity defined in `framework/spec_writing_guide.md` §Acceptance Item Granularity: group behavior variants around one behavior subject into one domain (error paths, boundary cases, and state transitions of the same subject are scenarios of that domain, not separate domains); do not split scenarios of the same domain into separate coverage requirements.
3. For each behavior domain, verify at least one acceptance item covers it (using the union input from step 1)
4. For each covered domain, verify the item's `implementation_surface` and `verification_surface` are consistent with the behavior's nature (e.g., REST API behavior should have surface `api`, not `db`)
5. If a behavior domain has no acceptance item → flag (possible untested behavior)
6. **Appendix behavior coverage check:** Extract all behavior domains, API contracts, data type definitions, and state machine transitions from appendix files — for contract content, apply the external-visibility boundary of `framework/spec_writing_guide.md` §4 first: internal field names, internal field layouts, internal timing — including retry/backoff values, internal data structures and their operations, internal function behavior, and configuration layout are design expression, not contract content, and create no coverage obligation. For each extracted domain or contract, verify there is at least one acceptance item in the main spec covering it. If an appendix describes contract or behavior content that has no corresponding acceptance item → **FAIL (actionable)** — the acceptance item set is the complete formal behavior carrier (see `framework/spec_writing_guide.md` §4), and contract content without item coverage is invisible to the cross-unit consistency check of every dependent unit. If appendix content directly contradicts an acceptance item (e.g., appendix says "timeout: 30s", item says "respond within 5s") → FAIL (actionable)
7. **Over-splitting detection (reverse check):** If multiple acceptance items satisfy the same behavior domain judgment (same behavior subject + same `verification_surface` + same `implementation_surface` + same `verification_type`), they are merge candidates → WARNING recommending a merge into one item. Merge method: keep one item id, delete the rest — the surviving id's process evidence stays valid. Items differing in `verification_type` are legitimate splits, not merge candidates.
8. **Orphan item detection (reverse check, FAIL):** For each design-driven acceptance item — an item whose `affects.appendices` does not reference the evidence appendix (Check 4 step 1) — verify the item's behavior subject (endpoint, function, state machine, or flow entry point, per the extraction premise in step 2) appears as a designed behavior somewhere in the candidate spec union: the main spec body or a non-evidence, non-exempt appendix. The subject may be designed as part of a larger flow; the test is subject presence, not narrative repetition of every contract element — the carrier obligation runs only one way, and contract elements are carried by the item itself under `framework/spec_writing_guide.md` §4. Exclusions: evidence-driven items are out of scope (their correspondence partner is the evidence appendix, enforced by Check 4); replacement/cleanup items are out of scope — `verification_type: inspectable` items whose `evidence_requirements` include `old_code_deleted` and `no_remaining_refs` are round-transitional verification requirements, not behavior declarations (the Check 4 step 1 replacement signal). If the subject appears in no designed behavior → **FAIL (P1, actionable):** retire the item together with its surviving narrative and appendix content (`framework/spec_writing_guide.md` §9 Cleanup obligation), or restore the behavior's design if it was dropped by mistake.
9. **Extraction evidence (required for every uncovered-domain and orphan-item FAIL):** Each uncovered-domain finding (steps 3, 5, and step 6's uncovered-content case) and each orphan-item finding (step 8) must carry an extraction artifact that makes the claim falsifiable:
   - **Source quote:** for an uncovered domain, the section heading and quoted text in the spec body or appendix that declares the behavior domain; for an orphan item, the item id and the quoted subject terms from its `description` / `pass_condition`
   - **Granularity judgment:** for an uncovered domain, why the quoted text is formal behavior (per the extraction premise in step 2, including the §4 external-visibility boundary applied to both body behavior domains and appendix contract content) rather than non-constraining narrative or internal design detail, and why its behavior variants form one domain (per the four granularity conditions in `framework/spec_writing_guide.md` §Acceptance Item Granularity) rather than scenarios of an already-covered domain; for an orphan item, why the quoted terms name a formal behavior subject of this unit (endpoint, function, state machine, or flow entry point) rather than a scenario of a designed domain or a dependency's behavior
   - **Absence claim:** for an uncovered domain, the covered surface checked (the union of every item's `description` and `pass_condition`, per step 1) and how the absence of any covering item was verified; for an orphan item, the surfaces checked (the main spec body and every non-evidence, non-exempt appendix) and how the absence of a designed behavior for that subject was verified
   A step-6 contradiction finding (appendix content contradicting an acceptance item) carries the two-sided quoted evidence step 6 itself requires — it is a conflict claim, not an uncovered-domain claim, and is not subject to this template.
   A finding without this artifact is not presented — the independent cross packet re-verifies the artifact before classification and marks unfaithful claims suppressed (a subject actually mentioned in an item, or variants split out of a covered domain, or a designed behavior whose subject matches an allegedly orphaned item — see §Step 9 → Extraction re-verification).

**PASS:** All behavior domains (main spec + appendices) have corresponding items with appropriate surface fields; every design-driven item corresponds to a designed behavior; no merge candidates found

**WARNING:** Merge candidates (over-split acceptance items)

**FAIL:** Uncovered behavior domain or surface type mismatch (actionable); orphan design-driven item with no corresponding designed behavior (step 8, actionable); appendix contract/behavior content without item coverage (actionable); appendix-main spec contradiction (actionable); contract statement without carrier coverage (5h, actionable); item violating the Contract Substance Baseline (5i, actionable)

**Check method:** Spec body + appendices × acceptance item set bidirectional cross-reference (body → items for coverage; items → body for orphan and over-splitting detection)

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
   - Verify absence of test double/fixture leakage (Anti-Pattern C): Description must NOT hardcode test doubles, mock runner names, or transient test fixtures (e.g. `mockRunner`, `test_tool`) into formal Given/When/Then scenarios — formal acceptance items must describe domain interactions, not test harness artifacts (see `framework/spec_writing_guide.md` §14.2 Anti-Pattern C). Leaking test fixtures → FAIL (actionable: restate using domain roles)
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

**FAIL:** One or more testable items lack sufficient descriptive detail for test derivation or leak test doubles/fixtures (actionable)

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
   A finding without this artifact is not presented — the independent cross packet re-verifies the artifact before classification and marks unfaithful claims suppressed (a value actually carried, or a statement reclassified as internal detail — see §Step 9 → Extraction re-verification).

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

**Contract substance vs. over-specification boundary:** Specific values mandated by S1 and S2 must represent genuine Contract Anchors owned by this unit or public contracts exported by dependencies per `framework/spec_writing_guide.md` §14 (Truth Ownership Framework). Hardcoding private implementation details, arbitrary wall-clock sleep durations, or collaborating unit unexported fields does NOT satisfy contract substance — it violates abstraction boundaries and is flagged under Check 2 and Check 7.

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
8. **Cross-Unit Private Implementation Leakage (Anti-Shadowing Rule, FAIL):** Enforce the Truth Ownership Visibility Law (`framework/spec_writing_guide.md` §14.1 and §14.2 Anti-Pattern A). A non-owner unit must never enumerate volatile, unexported, or private internal parameters/fields of collaborating units.
   - For every attribute, parameter, field name, or payload structure cited by the candidate spec regarding a dependency unit, verify that it resolves to one of:
     - **Public Contract Anchor:** An exported public contract symbol or explicit schema property declared in the dependency's formal behavior carriers (acceptance item set or protocol appendix), such as `contracts.SpanAttr*`, standard exported protocol events, or public REST/gRPC response keys.
     - **Behavioral Specification:** A behavioral description that does not hardcode unexported parameter names (e.g. "records delegation goal and child execution identifier" instead of literal private parameter names `goal, role, child_run_id`).
   - **Transitive Penetration Check:** If the candidate spec cites internal fields or types of an indirect dependency (a unit not declared in `unit_refs`), verify whether an explicit contract path exists. Citing internal fields of undeclared transitives without an exported contract anchor is a boundary violation.
   - If a spec mirrors or hardcodes unexported, private, or volatile internal fields of another unit:
     - **FAIL (P1: Shadow specification)** (actionable: replace private field enumerations with public contract anchors or behavioral specifications).

**Dependency scope report:** Report the carrier regions actually depended on — the dependency unit's acceptance item set and the whole protocol appendix files. The item set is declared as a **structural region dependency** (`acceptance_items` in the packet report's `Dependency scope:` lines; `specflowctl gate-evidence --acceptance-items` inspects the same region, see `framework/validation_cache.md` §Structural Region Dependencies): it is located by structure, so prose edits elsewhere in the same file — even inside the same content-defined chunk — do not stale this cache. When the no-contradiction assertion also reads a dependency unit's contract section (e.g. a protocol description in a named `##` section), declare that section region too (`sections` heading declaration). Protocol appendices are contract files and are declared whole.

**PASS:** No contradictions across related units; acknowledged contract changes are declared; no shadow specifications found

**WARNING (step 7):** Dependency unit carriers carry no comparable contract statements — consider enriching them per the Contract Substance Baseline

**FAIL:** Contradiction found (actionable), shadow specification / private implementation leakage found (actionable), or unacknowledged contract breakage (needs_decision)

**Check method:** Candidate × related candidates × related stables three-way cross-reference over the formal behavior carriers (acceptance item sets + protocol appendices).

---

## Check 8 — Constraint alignment

**Purpose:** The candidate design must not violate global constraints or bound rules.

**Execution steps:**

1. Read the stable global rule set (`docs/specs/rules/stable/g_rule_*.md`) and each bound rule listed in `rule_refs`. Stable global rules apply to every current-layer unit by default and are not repeated in `rule_refs` (see `framework/spec_writing_guide.md` §5). Execute the circular-dependency and layer-order prohibitions exactly as recorded in `g_rule_repository_baseline.md` §6.1 items 4-5 — the rule file carries the graph derivation, tooling cross-check, fail-closed behavior, and resolution path.
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

## Check 9 — Surface ownership & sharing

**Purpose:** The unit's declared code surface — the union of every acceptance item's `implementation_surface` and `affects.files`, directories expanded — must contain only files this unit implements. A code file belongs to at most one current-layer unit's surface; cross-unit reliance travels through `unit_refs`/`affects.dependencies` and the dependency's formal behavior carriers, never through declaring its files; shared constraint content lives in rules, and shared code lives in a unit. The rules are `framework/spec_writing_guide.md` §14.4 Surface Ownership.

**Execution steps:**

1. **Mechanical audit.** For a candidate target, run `specflowctl validate candidate --unit {unit}` and read its `Surface ownership` check line; for a stable-only target run `specflowctl surfaces` and locate this unit's entries. The repo-wide picture — every overlap, every directory declaration with its expansion size, and each out-of-bounds entry's `unit_refs` status — comes from `specflowctl surfaces` (`--json` for machine output). The mechanical result is deterministic tooling output, not a judgment: this check judges the resolutions. **Declaring the audit inputs:** in the dependencies packet report, check-9 declares the ownership surface it read — one `unit:{name}: frontmatter` line and one `unit:{name}: acceptance_items` line for every current-layer unit main spec the audit scanned (all peer units, not only `unit_refs` dependencies), plus this unit's own regions it judged. These declarations are what make another unit's declaration change stale exactly this check — an undeclared peer would leave the ownership judgment unverifiable (declare-heavy principle, see `framework/validation_cache.md` §Dependency Declaration).
2. **Overlaps.** For every reported overlap (a file this unit declares and one or more other current-layer units also declare):
   - Identify the file's true owner — the unit whose behaviors it implements. When it is plainly the other unit's file, the finding is `actionable`: drop it from this unit's fields and reference the owner through `unit_refs`/`affects.dependencies` when this unit relies on it.
   - When the file genuinely carries several units' behaviors, the resolution is a structure decision — reassign the behaviors to one owner, extract the shared code into a unit of its own (`framework/spec_writing_guide.md` §14.4 laws 2 and 5), or merge the shared constraint content into a rule when only constraints are shared — and is `needs_decision`.
   - Never resolve an overlap by keeping the duplicate declaration with an explanatory note: overlap is the defect, not a variant.
3. **Directory declarations.** For every directory declaration the audit reports for this unit, verify it names the unit's own implementation package and its expansion contains no other unit's declared files. A repository-wide or shared-boundary tree (`internal/`, `contracts/`, `runtime/`) is not a valid unit surface — narrow it to the files the unit actually implements (law 3). A directory whose expansion crosses another unit's files is the same overlap finding as step 2, reported once per owning file; the fix is the same ownership resolution.
4. **Shared constraint content.** For protocol/boundary content this unit states that is shared with other units, verify it lives in a rule (a new rule or a merge into an existing one) and is bound through `rule_refs`; a restatement is `actionable` to move into the rule, and `needs_decision` when the rule does not exist yet (extraction is a structure change). This step complements Check 8: Check 8 verifies the unit complies with its rules, Check 9 verifies shared truth was not re-declared beside them.
5. **Boundary declarations are not surface claims.** When the spec's prose assigns a behavior to another unit, that statement is a boundary declaration — it does not put the other unit's files into this unit's fields, and it is not itself a finding.

**PASS:** No overlap, no cross-unit directory expansion, and no shared-constraint restatement involving this unit

**FAIL:** Any unresolved overlap or cross-unit directory expansion involving this unit (`actionable` when the file is plainly another unit's and can be dropped; `needs_decision` when the resolution requires an ownership or structure decision) — P1; shared constraint content restated instead of carried by a rule (`actionable`, or `needs_decision` when the rule must be created) — P1

**Check method:** Mechanical ownership audit (`specflowctl validate` Check 9 / `specflowctl surfaces`) + ownership and sharing judgment over the spec (`framework/spec_writing_guide.md` §14.4)

---

## Step 9 — Write validate cache (tooling finalize)

After all local check packets complete, the independent cross packet re-verifies the extraction artifacts of Check 2 Step 1 and Check 5 FAIL findings (sub-check 5a step 9 / sub-check 5h step 4), performs the three validate cross-checks, and publishes the complete effective check-status/finding synthesis. `gate-finalize` then decides the cache mechanically from that accepted synthesis:

==ATOM_BEGIN:cache_evidence_path_forms==
**Declaring cache evidence — path forms:** spec objects resolved by name other than the run's own target files are declared as **logical references** instead of physical paths — `unit:{name}` for a unit main spec, `unit:{name}:appendix:{file}` for a unit protocol appendix (the full appendix file base name without `.md`, e.g. `unit:auth:appendix:unit_auth_account_token_claims`), `rule:{id}` for a rule file — with the `hash` + `deps` of the file actually read. The run's own target files (the unit's own main spec and appendices; for a rule target, the candidate rule file and its stable sibling) and code files keep physical paths. A logical reference resolves at freshness time to the current-layer file (candidate first, stable fallback), so promoting the referenced unit or rule does not stale a cache whose dependency content is unchanged (see `framework/validation_cache.md` §Logical References).
==ATOM_END:cache_evidence_path_forms==

### Extraction re-verification (Check 2 Step 1, Check 5)

Step 1 goal-means findings (Check 2), uncovered-domain and orphan-item findings (sub-check 5a, step 9 artifact), and uncarried-contract findings (sub-check 5h step 4) carry an extraction artifact. Before classification (cross synthesis), the independent cross packet re-verifies each artifact with deterministic checks:

- Sub-agent claims a Check 2 Step 1 flag holds ("the flagged behavior serves no stated goal", "the described behaviors cannot meet the goal", "the behavior violates a non-goal", or "a smaller design surface would achieve the same goal") → re-read the goal or non-goal declaration and the flagged behavior or design surface; a quote that does not establish the claimed relationship — including a proportionality claim resting on preference rather than an establishable smaller surface — drops the finding
- Sub-agent claims "no item covers behavior subject X" → re-read the item set (the union of every item's `description` and `pass_condition`) and confirm X's quoted subject terms are really absent; a subject actually mentioned, or behavior variants split out of a covered domain (granularity violation), drops the finding
- Sub-agent claims "no designed behavior matches item X" (orphan item) → re-read the spec union (main spec body and every non-evidence, non-exempt appendix) and confirm X's quoted subject terms are really absent as a designed behavior; a designed behavior whose subject matches, or terms naming a scenario of a designed domain, drops the finding
- Sub-agent claims "no carrier states contract value Y" → grep the item set and the protocol appendices for the quoted value; a value actually carried at comparable granularity drops the finding
- Sub-agent claims "Z is a behavior domain" (5a step 2), "Z is a contract statement" (5h), or "Z is appendix contract content requiring acceptance coverage" (5a step 6) → check the classification against the external-visibility boundary of `framework/spec_writing_guide.md` §4 (externally-observable behavior vs internal design detail); content reclassified as internal detail drops the finding

Re-verification failure → the cross result marks the finding `suppressed` and publishes the affected check's effective PASS status if no retained finding supports FAIL. This runs before classification because a post-execution check re-run cannot detect a wrong-direction fix. Every suppression remains visible in the cross audit artifact with its location and reason; only retained findings enter the generated user report and counts.

- **If all checks PASS (after re-verification):** the packet run writes the validate cache per `framework/validation_cache.md` format:
  - `gate-finalize` creates `docs/specs/meta/validation/unit/{name}/` as needed
  - Collect dependency evidence for every file read during validation, including:
    - Main spec file
    - Every non-exempt appendix file
    - All referenced files (unit_refs, rule_refs, affects.files are already included)
  - Each packet report declares its files' dependency scope in its `Dependency scope:` lines (`sections` / `ranges` / `acceptance_items` / `acceptance_item:<id>`); `gate-submit` validates path membership against that packet's `read_refs` (not merely the run snapshot), and `gate-finalize` computes the `hash` + `deps` evidence from the accepted reports. The values come from the sub-agent's per-check `Dependency scope` report (see §Output Format); a file reported as `all` (or not reported) is declared as a whole file. The declared ranges must cover every region the validation judgment depended on — when unsure, declare more (declare-heavy principle; see `framework/validation_cache.md` §Dependency Declaration). **The unit's own main spec is declared by section regions, not line ranges:** the per-check `sections` values become the cache entry's `checks` mapping (check key = the agent check number), with the union in `deps` (see `framework/validation_cache.md` §Format → Per-check evidence); the tooling enforces union discipline. Validate's acceptance coverage judgment (Check 5) reads the item set as a whole and declares the whole-set token `acceptance_items`; item-level `acceptance_item:<id>` declarations are verify's per-item mode.
  - The `gate-finalize` write produces `validate_result.md` with `result: pass`, `target: candidate`, `mode: full`, file hashes and dependency CIDs, assembled from the accepted packet reports.
  - Targeted runs (`:check-{n}` / `:{keyword}`) never write a cache, and a targeted run that FAILs deletes a pass cache (a failure record is kept — it is already blocking and is the recovery baseline) — any FAIL at any granularity means promote must not proceed — see `framework/validation_cache.md`

- **If any FAIL remains (after re-verification, full run, candidate target):** `gate-finalize` writes a failure record (`result: fail` + `blocking: true`, `mode: full`, `basis: full`, severity counts, findings body, and the per-check `status` map — `pass`/`fail` for every executed check plus the cross-check; full runs have no `carried`) — the failure-recovery baseline for `revalidate@{unit}`. Promote must not proceed. Proceed to Present Findings. (A stable-only full FAIL writes the same record shape — see §Stable-only mode.) After the findings are resolved, `revalidate@{unit}` (repair) re-checks the failed checks, persisted invalidated checks, newly affected checks, current keys the baseline never declared, and explicit `--rerun` overrides, then carries the passed checks over — each carried `pass` was executed as a complete independent judgment and its declared evidence is unchanged (see §Delta re-run and §Failure handling by gate role in `framework/validation_cache.md`). During the fix iteration, targeted re-checks (`validate@{unit}:check-{n}` / `:keyword`) remain the lightweight local option.

### Delta re-run (revalidate@{unit})

Candidate targets, plus stable-only targets with a usable baseline — a delta re-run against a stable-only target whose confirmation cache is MISSING reports the missing-baseline message — "No usable confirmation baseline. Run the full `{command}@{target}` (confirmation check) first, or `specflowctl fork {--unit|--rule} {name}` to start a new round." — and stops (see `framework/verification_scope.md` §Delta Runs → Layer applicability). Triggered by the user when the cache is STALE or BLOCKED (a failure record). Follow §Delta Runs in `framework/verification_scope.md`:

1. **Preconditions** — the existing cache must have `mode: full` and one of the two baselines: `result: pass` (stale-cache recovery) or `result: fail` + `blocking: true` (failure-record recovery, §Failure recovery — the record written by a candidate full-run FAIL or by a delta FAIL). A MISSING cache has no usable baseline — run the full command instead.
2. **Plan (mechanism-derived)** — run `specflowctl gate-plan --gate validate (--unit {name} | --rule {id}) --target {candidate|stable} --mode delta` (or `--mode repair` for a failure record). The planner derives the re-run set from the cache's per-check `checks` mapping — affected = {checks whose declared deps went stale} ∪ {current checks the baseline never declared} ∪ {persisted `invalidated_checks`} ∪ {explicit `--rerun` overrides} ∪ {cross-check} — maps unclaimed deps by the fixed associations (a `unit:` entry → Check 7; a `rule:` entry → Check 8), and reports the re-run packets, carried-over checks (`carried_keys`), and any degradation or full-scope coverage statement. After a targeted P0/P1, the coordinator runs `gate-invalidate --check {check key}` immediately; repair reads that state automatically and never carries the contradicted judgment. Legacy caches without per-check evidence and invalidated keys that cannot map to the current surface degrade to the full packet set. The plan is the scope; the executor does not re-derive or extend it.
3. **Execution** — execute the planned packets (protocol: `framework/verification_scope.md` §Gate Work Packets; packet reports carry the `Dependency scope:` lines per §Output Format), submitting each report with `specflowctl gate-submit --run <run_id> --packet <packet_id> --report PATH`. Any P0/P1 finding during the re-run makes the finalize write a **failure record** (`result: fail`, `blocking: true`, severity counts, findings body, `mode: full`, `basis: delta`, and the per-check `status` map — `fail`/`pass` for the re-run checks, `carried` for the carried-over ones, derived by `gate-finalize` from the accepted packet verdicts). Stop, present findings (same as full FAIL — Check 2 Step 1 and 5a/5h findings re-verified per §Step 9 → Extraction re-verification before presentation).
4. **On PASS** — `gate-finalize` rewrites `validate_result.md` with `mode: full`, `basis: delta` (or `basis: repair` when recovering from a failure record), `target: candidate` (stable-only target: `target: stable`), a fresh `timestamp`, and a **complete** `files` list: new `hash` + `deps` + per-check `checks` evidence (computed by the tooling from the accepted packet reports) for the re-run checks' files, and the original evidence for the carried-over checks' files, merged by the tooling from the baseline cache (their CIDs are unchanged by construction). Logical references (`unit:{name}` / `unit:{name}:appendix:{file}` / `rule:{id}`) are carried over the same way.

---

## Check 10 — Reader contract

**Purpose:** Enforce the Reader Contract (`framework/spec_writing_guide.md` §9) under reconstruction conditions: an independent reader who has never seen the code and holds no author context must be able to reconstruct the design from the spec's human-readable part alone. The check splits the labor explicitly: machine validation keeps locality (the declared reading scope, the report structure, the carrier evidence set), while global coherence — whether the parts compose into one design — is kept by closed-book reconstruction followed by independent reconciliation. There is no fixed question bank on purpose: per-question answerability is a constructive-local property, and locally valid answers do not compose into a design (the collage counterexample — text whose every paragraph is locally sound but which shares no single design — passes any question-answering check by construction).

**Human-readable part (reading scope):** every `##` section of the unit main spec before the section containing the `acceptance_item_set:` marker. The acceptance-enclosing section itself and everything after it, appendices, and code are outside the reader's scope — a design narrative buried after the acceptance section or inside an appendix does not satisfy the contract. The gate plan computes the section list mechanically (the reader packet context carries it verbatim); a spec without the marker section, or with no narrative section before it, fails closed at plan time.

**Packet structure:** Check 10 owns two packets that always execute together — the `reader` packet (closed-book reconstruction) and the `verifier` packet (reconciliation, depends on the accepted reader result). The reader authors evidence only (the reconstruction and the Undetermined list); the verifier authors classifications only (Claim, Carrier, Must-close, Consistency lines); gate-submit composes the check verdict and the findings mechanically from the classifications. Judgment enters the gate only at the classification step — the verdict itself is never authored. Neither packet's reviewer writes finding entries.

### Reader packet — closed-book reconstruction

The reader is an independent read-only session (packet mission; §Sub-agent Prompt Assembly in `framework/verification_scope.md`). It reads only the human-readable part — no acceptance items, no appendices, no code — and receives no question bank. It produces one contiguous design restatement and the honest list of what it could not determine:

```
Reconstruction:
{one contiguous prose block: what the unit is, how the main path works,
 where state lives, how failure is exposed, what the boundaries are,
 what it produces}

Undetermined:
- {anything that would not connect or had to be guessed}
```

- The restatement must be one non-empty contiguous block. No quotes and no citations are required: quoting proves local answerability, and local answerability does not compose into a design — the restatement must carry the connections itself.
- `Undetermined` is the honesty trace: every place the reader had to guess or could not connect is declared instead of filled. Declare exactly `- none` when nothing is undetermined; `none` never combines with entries.
- The reader must not write a verdict line or finding entries — the check verdict is the verifier packet's, composed mechanically.

gate-submit validates the report mechanically: the Reconstruction block is present, contiguous, and non-degenerate; the Undetermined list is well-formed; and the declared Dependency scope sections are exactly the human-readable section list. Any violation rejects the submission (the executor corrects the report; every attempt is kept).

### Verifier packet — reconciliation

The verifier is a second independent read-only session. Its inputs: the accepted reconstruction (embedded in its packet mission), the human-readable part (the support source), and the formal carrier — the acceptance item set and the protocol appendices (the verifier's packet context carries the carrier backbone; its read refs cover the main spec and the appendices). It reports the check verdict, one line per classified claim, one Carrier line per acceptance item, one Must-close line per §9 decision, and one Consistency line. The seven §9 must-close decisions keep their fixed ids, identical to the tooling's decision table:

| ID | Decision |
|----|----------|
| Q11 | which object owns a responsibility |
| Q12 | which entry point starts the behavior |
| Q13 | where state or durable truth lives |
| Q14 | how ordered steps connect |
| Q15 | how boundary failures are reported |
| Q16 | what the result shape means |
| Q17 | how acceptance proves the stated responsibility |

```
10. Reader contract: PASS | FAIL — {reason}

Claim: C01 = supported — {basis}
Carrier: {acceptance item id} = seen — {basis}
Must-close: Q11 = closed — {basis}
Consistency: coherent — {basis}
```

Claim classification (this is the step where judgment lives, and the only one):

- `supported` — the human-readable part supports the claim.
- `contradicted` — the formal carrier contradicts the claim; the basis records both sides so the user can adjudicate.
- `unsupported-central` — the claim's substance exists in the formal carrier but the human-readable part never states it, and the claim touches a central mechanism. This is a document gap.
- `unsupported-minor` — same, but the claim is a minor detail. Advisory only: presented on the check line, never a machine finding.
- `reader-error` — neither the human-readable part nor the carrier supports the claim: the reader invented it. It does not affect the verdict — the document is not condemned for the reader's fabrication (re-running the reader is the remedy).

**Central** = the unit's declared responsibility + each acceptance item's behavioral subject + every applicable must-close decision. The carrier mapping works in the opposite direction — from the formal carrier into the reconstruction: `seen` — the item's behavioral subject is reconstructable from the restatement; `missing` — neither the restatement nor the Undetermined list lets the verifier see it, meaning the document did not let the reader see it. A subject the reader could only guess at is not a reconstructed subject: `Undetermined` entries inform the verifier's judgment, but a guessed design is still `missing`. Must-close mapping: `closed` — the reconstruction carries the decision; `missing` — it does not; `not-applicable` — the decision has no subject in this unit, with the basis stating why.

**Declarative units** (type/contract units produced by §14.4's shared-code law) are reconciled by their declared responsibility and their acceptance items' behavioral subjects — the public surface and whom it serves, the form things take crossing the boundary, what is frozen, what breaks when changed — never by a behavior template. A declarative unit does not need a flow to pass; it needs its shape to be reconstructable.

The verdict must equal the mechanical outcome: `FAIL` exactly when at least one claim is `contradicted` or `unsupported-central`, at least one Carrier line is `missing`, at least one Must-close line is `missing`, or `Consistency: incoherent`; otherwise `PASS`. Every blocking classification composes exactly one mechanical P1 finding (actionable, document-level): a `contradicted` claim (reconcile the narrative with the carrier; the user adjudicates from the recorded sides), an `unsupported-central` claim (state the substance in the narrative), a `missing` Carrier line (let the narrative carry the item's behavioral subject), a `missing` Must-close line (close the decision or declare its open boundary per §9), and an incoherent consistency (resolve the self-contradiction). `unsupported-minor` claims enter no finding — validate grades findings P0/P1 only.

### Verdict and severity

Check 10 `FAIL` findings are P1 (contract-decided default) and gate-driving — they block promote like every other validate P0/P1. The fix is always actionable at the document level: state the missing substance, close or bound the decision, or reconcile the contradiction, in the human-readable part.

### Design boundary and calibration

The split of labor is part of the contract: machine checks keep locality (the human-readable boundary, the declared reading scope, report structure, carrier evidence); global coherence is kept by the independent reconstruction and reconciliation pair. Two structural-fact probes once proposed for this check were rejected and must not be reintroduced as gate criteria: a single-home rule (each fact written in exactly one place) and a linear-path rule (no cross-section jumps) test layout, not semantics — a collage passes both — and the single-home rule conflicts with declarative units, whose narrative must restate shape facts the acceptance items also carry. Calibration is a periodic meta practice, not a gate dependency: sample accepted reconstructions and their classifications by hand to estimate the false-pass rate; a verifier that has never produced a blocking classification over many runs is an anomaly signal worth auditing.

### Scope and lifecycle

- **Delta/repair:** check 10's dependency evidence is the union of both packets' declared evidence — the human-readable section set (reader and verifier), the acceptance item set, and the protocol appendices (verifier). A narrative edit, an acceptance-item edit, or a carrier-appendix edit re-runs the reader and verifier packets together: the carrier backbone changed, so the reconciliation baseline changed. Rule changes and dependency changes carry it over. The check never carries partially — both packets re-run or neither does.
- **Stable-only targets:** the probe runs against the stable main spec like every other check (the boundary resolves from the stable file's own `acceptance_item_set:` marker).
- **Retiring units:** skipped entirely — no reader or verifier packets are planned (see the retiring-unit note in Check 1).
- **Empty acceptance item set:** the probe is skipped like a retiring unit's — with no carrier there is nothing to reconcile against. Validate still plans the run so Check 2 can report the empty set; the gate fails on that check, so the skip cannot produce a passing record.
- **Targeted runs:** `validate@{unit}:reader` (or `:check-10`) executes the same reconstruct-then-reconcile protocol as two independent subagent sessions coordinated by the main agent, without a packet run or cache write; a P0/P1 finding must first be persisted with `gate-invalidate --check 10`.
- **Boundary-move note:** reordering sections across the acceptance boundary without editing their content does not stale the check's declared evidence (section CIDs are unchanged). Such a reorder is a deliberate structural change (§13 naming stability) and is the author's declared-change responsibility, like retirement itself.

---

## Present Findings

Advisory findings (Check 1 step 7 hygiene WARNING, Check 2 Step 4 taste-level P2/P3, Check 3 step 6 split-candidate WARNING, Check 5a step 7 merge-candidate WARNING, Check 7 step 7 carrier-substance WARNING) are presented for awareness only — they enter neither the batch group nor the decision group, need no decision, and do not block the flow. They are presented on their check line's reason even when all checks PASS. Each Check 2 Step 4 advisory finding records its §9 confirmation in the same check-line trace (`confirmed` / `adjusted: {Px} → {Py}` with evidence). Advisory findings never produce machine `Severity confirmation:` records and must never be emitted as bracketed findings — validate grades findings P0/P1 only, and a retained P2/P3 finding rejects the run's synthesis at `gate-finalize`.

### Batch classification (validate)

When FAIL items exist, the main agent classifies each finding into a **batch group** or a **decision group** before presenting. Classification uses each FAIL's resolution type (actionable / needs_decision) and the check definition — it adds no new analysis; for batch group candidates it only runs lightweight assertion re-verification (see Assertion re-verification below).

**Batch group eligibility:** A finding enters the batch group only if its fix is fully determined by an objective standard — no interpretation of design intent is required. The batch group is limited to these fix types:
- Check 1: missing required frontmatter fields (standard: the required fields list)
- Check 1: unit_refs / rule_refs / appendix references to non-existent files (standard: file existence)
- Check 1: appendix path or naming not following the convention (standard: the path convention)
- Check 5d: testable item description missing Given/When/Then (standard: the Gherkin-style convention in `framework/spec_writing_guide.md`)

All other FAIL findings — including 5e/5f/5g/5i content rewrites, Checks 2/3/4/6/7/8, and every needs_decision item — go to the decision group. **needs_decision items always go to the decision group.**

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

validate grades findings P0/P1. In a packet run, the cross report carries one complete mechanically parsed severity-confirmation sequence for every terminal retained finding, including the contract-decided P1 default (its `confirmed` record states the contract-decided grade stands; no boundary check applies); this uniform coverage prevents an unconfirmed severity from reaching `gate-finalize`. A P0 grade is a judgment-based assignment and its evidence must establish the boundary in `framework/severity_policy.md` §9. Read the impact surface the grade depends on (the downstream consumer, dependent unit, or the section/appendix the claim relies on), and publish the exact record syntax owned by `framework/verification_scope.md` §Gate Work Packets → Packet report contract. A confirmed first result completes the sequence; an adjusted first result requires exactly one final second record. Check 2 Step 4 advisory findings are confirmed under the same §9 boundary rules, but their records stay in the check-line trace (see Check 2 Step 4) — they are not packet `Severity confirmation:` records and are not emitted as findings. Targeted runs keep the same human-readable trace but do not create packet state.

```
Severity check:
  confirmed: N | adjusted: N
  Severity confirmation: {finding_id} = confirmed {Px} — evidence: {file}; reason: {one line}
  Severity confirmation: {finding_id} = adjusted {Px} -> {Py} — evidence: {file}; reason: {one line}
  Severity confirmation: {finding_id} = confirmed {Py} — evidence: {file}; reason: {required final second check after an adjustment}
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
Severity check:
  confirmed: N | adjusted: N
  Severity confirmation: {finding_id} = confirmed {Px} — evidence: {file}; reason: {one line}
  Severity confirmation: {finding_id} = adjusted {Px} -> {Py} — evidence: {file}; reason: {one line}
  Severity confirmation: {finding_id} = confirmed {Py} — evidence: {file}; reason: {required final second check after an adjustment}
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

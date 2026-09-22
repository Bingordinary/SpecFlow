# Spec Review Checklist

## Overview

When an agent executes `review@{unit}`, it uses the spec-aware code quality review defined in this file. This file is referenced by `framework/concepts.md` — the agent reads this file at review time, not proactively.

## Mode Selection

| Trigger | Mode | What to execute |
|---------|------|-----------------|
| `review@{unit}` | full | Read all files referenced in the candidate spec's `affects.files` and `implementation_surface` across all acceptance items → review those files using the standard defined below |
| `review@{unit}:{keyword}` | targeted | Match keyword to a file name in `affects.files` or `implementation_surface` → review that file using the standard defined below. Does not write a cache. |

**Keyword domain:** review keywords resolve to code file names from the candidate spec's `affects.files` or `implementation_surface` (e.g., `:login.go` → review `src/auth/login.go`). A feature-name keyword is mapped to the files behind that feature. A keyword matching no file is a no-match — ask the user for clarification.

**Output:** Targeted runs report only the requested file(s) and note "This was a targeted check — no complete cache was written. Run `review@{unit}` for a complete review." A targeted P0/P1 must first be persisted with `gate-invalidate --check {file}`.

**Cache:** see `framework/validation_cache.md` for format.

## Execution Rules

- **Subagent permissions:** may inspect file content, search text by pattern, locate files by name pattern, and query git history. Must NOT modify files or execute commands that change state.
- **Cross-check:** See §7.
- **Failure Behavior:** If subagent encounters an error (cannot read target files, target unit not found, review checklist missing), report "Review could not complete — <reason>". Do not write review cache. Advise resolving the issue before retrying. This is distinct from review findings — when the review runs and finds P0/P1 issues, the subagent completed normally (the output is PASS or FAIL per the gate rules below), not a subagent failure.
- Each finding reports P0-P3 severity with code references.
- Suppressed findings are listed separately under "Suppressed by spec".
- **Sub-agent prompts:** prompts for review sub-agents must point to the spec file for design context, not restate it — a restated prompt is a second spec source, and suppression decisions must be read from the spec itself. File lists for review sub-agents come from `specflowctl next --unit <name>` output (implementation surface + affects files), following the sub-agent prompt assembly rules in `framework/verification_scope.md` §Sub-agent Prompt Assembly — with the role line "read-only review sub-agent for review packet {packet_id} of run {run_id}" and the protocol reference pointing to this checklist (`framework/spec_review_checklist.md`).

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
- `Result` — `PASS | FAIL` for all three commands. For packet runs this value is generated by `gate-finalize` from the accepted synthesis result (or from the single rule-validate packet, which has no cross packet); the coordinator never supplies it. The gate is decided by severity: FAIL means retained P0/P1 findings exist. validate grades findings P0/P1 only, so any validate FAIL is a P0/P1 finding; verify/review FAIL means retained P0/P1 mismatches/findings exist.
- `Blocking promote: yes | no` — `yes` when P0/P1 findings exist (the run FAILs); `no` otherwise. Valid for all three commands.
- `Key counts: Findings: N (P0: a | P1: b | P2: c | P3: d)` — N is the retained finding count and a/b/c/d are generated by `gate-finalize`; the coordinator never supplies them. validate grades findings P0/P1 only, so its P2/P3 counts are always 0; verify's blocking mismatches equal the P0/P1 counts and non-blocking mismatches equal the P2/P3 counts. Command-specific summary numbers (validate's failed checks and advisory findings, verify's coverage, review's suppressed-by-spec count) appear in the body.
- `{body}` — command-specific content defined in this file's body format section (validate: one line per check; verify: Items / Scope / Integrity / Coverage / first-principles divergence analysis; review: Architecture assessment and suppressed findings).
- `Findings:` — `Findings: none` when no finding is retained; otherwise one entry per finding: a unified entry line `[{severity}] {location} — {issue} (actionable | needs_decision)` followed by the finding block's required detail lines. `actionable` — a concrete repair can be made without user judgment (verify: direction spec_gap/code_gap; review: a determined recommendation); `needs_decision` — requires user input or a design decision before the fix can be made (verify: direction needs_design/blocked; review: architecture trade-offs). The block is written for a reader who has not opened the spec or the code: delete every `ref:` line and every `§`, line-number, or other anchor reference from the field text, and the reader must still learn what is wrong, what the conflicting sides say, what is at risk, and what to fix or decide. Anchors appear only in the optional trailing `ref:` line, which carries tracking information and no meaning.
  - `problem:` — one sentence naming the concrete subject (acceptance item id, field or parameter name, behavior) and what is wrong with it; never an anchor, region name, or pointer.
  - `evidence:` — the quoted content that proves the claim, as one or more indented `- {verbatim quoted source content} — {source label}` sub-lines. Contradiction findings quote both conflicting sides verbatim; undefined or missing behavior lists the defined part and the missing part; spec-vs-code findings quote the declared behavior and the implemented behavior. A review P3 finding uses its `fact_anchor:` line as this evidence form.
  - `impact:` — what goes wrong, or stays undecidable, if the finding is not resolved.
  - `fix:` — actionable findings: the concrete repair action. `decision:` — needs_decision findings: the question the user must answer, with `options:` sub-lines listing the candidate choices.
  - The block's detail lines are contiguous indented lines directly under the entry line — no blank lines inside the block (the block is stored and re-rendered verbatim when a finding is carried into a delta/repair run).
  - `{command-specific detail lines}` — command-specific extras follow the shared fields: verify's `root_cause:`, `direction:`, and `confidence:`; review's `spec_context:` and its mechanically required `fact_anchor:` for P3 findings. validate and rule validate add none.
  - Findings are grouped into the batch group and decision group defined in this file's batch classification section when this file defines one; flat when this file defines no batch classification or grouping is inactive. Batch-group entries carry the same fields; terse one-line values are fine.
- `Dependency scope:` — one line per check the run executed: `{check key}: {file}: {declaration}`. `{check key}` is the command's check identifier (validate: `check-{n}`; verify: the acceptance item id; review: the packet's file path). `{declaration}` is the section-region heading text the check's judgment read (e.g. `Description`, `Testability / Acceptance Criteria`; the frontmatter region is `frontmatter`), `acceptance_item:<id>[,<id>...]` (one or more acceptance item regions of the spec's `acceptance_item_set` — the precise declaration for a judgment over specific items, e.g. one verify item judgment), the reserved token `acceptance_items` (the whole `acceptance_item_set` structural region — for judgments over the set as a whole, e.g. validate's acceptance coverage check), 1-based closed line ranges (e.g. `120-180,300-320`), or `all` when the judgment covered the whole file. Every read-only subagent reports this scope for the checks in its packet; the report is submitted verbatim via `gate-submit`, which validates the declarations against that packet's `read_refs` — not merely the run-wide snapshot — (path membership and declaration parseability), and `gate-finalize` computes the CIDs and records the per-check breakdown in the cache's `checks` mapping (see `framework/validation_cache.md` §Format → Per-check evidence). Delta/repair runs report the scope of the re-run checks only — carried-over checks are not re-executed and get no new declaration (see `framework/verification_scope.md` §Sub-agent Prompt Assembly Check / Packet scope). Targeted runs may omit it.
- `Severity check:` — packet-run reports use the exact `Severity confirmation:` line grammar owned by `framework/verification_scope.md` §Gate Work Packets → Packet report contract. Unit cross reports carry one complete sequence for every terminal retained finding after disposition/merge resolution: one `confirmed` record, or an `adjusted` record followed by exactly one final record for the adjusted severity. `confirmed: N | adjusted: N` counts findings by final sequence outcome, not record lines. Rule validate reports carry the confirmation sequences required by their command checklist. `gate-submit` parses and validates these records; `gate-finalize` uses the resulting canonical severities. Targeted reports retain the command checklist's human-readable severity trace but do not create packet state.
- `Incremental scope:` — delta runs only (mode `delta`). One line per re-run check in the run's own structure (e.g. validate: "check-5 (acceptance coverage & correctness): re-run — section `Description` of the unit's own spec changed"), followed by a line declaring the carried-over checks ("checks 1-4, 6-8: carried over — their dependency evidence is unchanged") and the cross-check result (unit targets — rules have no cross-check). The scope is mechanism-derived at plan time from the cache's per-check evidence: `gate-plan` maps stale regions to the checks that declared them, adds the current keys the baseline never declared (a new acceptance item or review file has no evidence to carry, so it executes like a stale judgment), and generates the re-run packet set (see `framework/verification_scope.md` §Delta Runs). For a failure-record recovery (`basis: repair` — the run recovers a delta FAIL's failure record or a full-run FAIL's record), the plan's packet set is the record's failed checks plus its persisted `invalidated_checks` (written by `gate-invalidate` after a targeted P0/P1), the newly affected checks, the current keys the baseline never declared, any explicit `--rerun` overrides, and the cross-check; carried-over checks are the remaining `pass`/`carried` entries (see `framework/verification_scope.md` §Delta Runs → Failure recovery). A failure record whose per-check status map is absent or incomplete (legacy or malformed), or whose invalidated key cannot map to the current judgment surface, degrades the plan to the full packet set — nothing is carried over. When the re-run covers every declared check, the plan covers the full scope — nothing is carried over. The incremental scope is reported by `gate-plan` before execution begins (the user must see what will be re-run and what will be carried over) and again in the final report.
- `Next step:` — the concrete command to run next with its reason; `None` when nothing further is needed. A finding's fix lifecycle has three states with fixed wording: `finding_open` → "Resolve the findings, then re-run the target-appropriate re-check command (`validate@{target}:check-{n}`; unit targets also `verify@{target}:{keyword}` / `review@{target}:{keyword}`) to confirm"; `fixed_pending_recheck` → "Fixes applied; re-run the target-appropriate re-check command to confirm." — only after the approved fix was actually written; `verified` → "Re-check passed." — only after a re-check confirmed the fix. A gate report is always produced before any fix is applied (nothing is implemented before the user approves the findings), so an actionable finding's report-time `Next step` is always the `finding_open` wording. Other guidance: all gates green → "if the design is finalized, run `promote@{target}`"; needs_decision → "awaiting your decision on {item}"; nothing further → `None`.

**Targeted runs:** end the report with the command's targeted note ("This was a targeted check — no complete cache was written. Run `{command}@{target}` for a complete ...") after the `Next step` line. If the result contains P0/P1, run `gate-invalidate` before reporting completion.

### Completion — Persist Gate Cache

A quality-gate run is complete only when its result is persisted and visible to `fresh`/`promote`. A report alone does not satisfy the gate.

- **Full (`validate@{target}` / `verify@{unit}` / `review@{unit}`, `mode: full`, `basis: full`)** — complete only when the packet run finished and its cache was written: (1) `specflowctl gate-plan --gate {validate|verify|review} (--unit {name} | --rule {id}) --target {candidate|stable} [--input PATH_OR_REF]...` ran **before any executor read input** (before prompt assembly / sub-agent launch) — `--input` adds evidence that every packet may read but never creates a work packet; (2) every required detection/check/review/analysis report was submitted via `specflowctl gate-submit --run <run_id> --packet <packet_id> --report PATH` and accepted (verify analysis packets are `not_required` when their detection packet is not `MISMATCH`); (3) the cross packet consumed the accepted packet results and was accepted (unit targets only; rule validate has one complete packet and no cross); (4) the coordinator ran `specflowctl gate-finalize --run <run_id>` and the cache `docs/specs/meta/validation/{unit|rule}/{name}/validate_result.md` | `verify_result.md` | `review_result.md` was written; (5) `fresh@{target}` (`fresh --unit {name}` / `fresh --rule {id}`) shows the expected gate state (`FRESH` for `result: pass` / `blocking: false`, `BLOCKED` for `result: fail` / `blocking: true`). A candidate validate **full-run** FAIL is the exception: `gate-finalize` deletes any existing validate cache and writes no failure record; candidate validate delta/repair FAIL writes the failure record required for recovery. A finalize that finds any input changed since `gate-plan` is rejected and writes no cache. See `framework/validation_cache.md` §Write Rules and §Failure handling by gate role.
- **Delta (`revalidate@{target}` / `reverify@{unit}` / `rereview@{unit}`, `mode: full`, `basis: delta` or `basis: repair`)** — the same packet sequence with `specflowctl gate-plan --mode delta` (or `--mode repair` from a failure record): the plan derives the re-run packet set mechanically and records the carried-over checks; `gate-finalize` merges the carried evidence from the baseline cache. See `framework/verification_scope.md` §Delta Runs.
- **Targeted (`:check-{n}` / `:{keyword}`, no complete cache)** — complete when the report is produced and, for P0/P1, the coordinator has run `specflowctl gate-invalidate` for every affected check key. Targeted runs intentionally do NOT create a gate run — `gate-plan` / `gate-submit` / `gate-finalize` are not run. A targeted PASS leaves cache state unchanged. A targeted P0/P1 deletes a pass cache or persists `invalidated_checks` on a failure record and invalidates any matching open run; it never publishes a complete result cache or satisfies the promote gate. The report ends with `This was a targeted check — no complete cache was written. Run {command}@{target} for a complete ...`.

**Self-check:** `ls docs/specs/meta/validation/{unit|rule}/{name}/` and `fresh@{target}` (unit: `fresh --unit {name}`, rule: `fresh --rule {id}`).
==ATOM_END:report_skeleton==

### Body format (review)

```
Architecture assessment:
  conclusion: acceptable | needs_attention | unacceptable
  module_boundaries: {assessment} — {basis}
  responsibility_organization: {assessment} — {basis}
  dependency_clarity: {assessment} — {basis}
  abstraction_level: {assessment} — {basis}
  extension_landing_points: {assessment} — {basis}
  engineering_patterns: {assessment} — {basis}
  gate_findings: none | [P0|P1] {finding} [; [P0|P1] {finding} ...]

Suppressed by spec (N):
  {location} — {issue} → {suppression_reason}, {spec_ref}
```

**`{layer}` (skeleton header):** the layer of the spec the review is based on — `candidate`, falling back to `stable` when no candidate exists (§2 Pre-review Setup).

**Architecture assessment (Dimension 8):** The assessment reports the code structure of the reviewed surface per Dimension 8 in §4 below. Gate-level architectural defects (P0/P1) appear both in the assessment's `gate_findings` and in the Findings section; advisory architectural judgments appear as P2 findings, while P3 is limited to objective findings that pass the §5 P3 reportability gate. A spec-recorded architectural decision with a conforming implementation is not re-questioned here. The `gate_findings` line carries the Dimension 8 P0/P1 entries themselves — `none` when there are none — so the conclusion mapping is mechanically checkable; a pointer such as "reported below" is not a valid value.

**Conclusion mapping:** any Dimension 8 P0/P1 gate finding → `unacceptable`; P2/P3 only → `needs_attention`; none → `acceptable`. The conclusion does not change the Gate Rules table — the gate remains decided by P0/P1 findings.

**Mechanical enforcement:** `gate-submit` enforces the mapping's P0/P1 half: a `[P0]`/`[P1]` entry in `gate_findings` requires the `unacceptable` conclusion; `unacceptable` requires both a P0/P1 `gate_findings` entry and at least one P0/P1 finding in the report; and the `gate_findings` line must be `none` or one or more bracketed P0/P1 entries. The P2/P3/none half (`P2/P3 only → needs_attention`, `none → acceptable`) is an execution-language instruction to the report author: Dimension 8 P2/P3 assessments have no machine carrier, so the tool cannot distinguish them from other dimensions' P2/P3 findings and does not enforce this half.

### Findings section

Each finding uses the unified finding format `[{severity}] {location} — {issue} (actionable | needs_decision)` followed by the shared finding block (§Output Format). The resolution label reflects the recommendation: a single concrete recommendation (rename, remove dead code, fix a call) → `actionable`; a recommendation that requires a user decision (architecture trade-off, accepting tracked debt) → `needs_decision`. The block's `fix:` (actionable) or `decision:` (needs_decision) states the repair or the decision; `spec_context:` (design context, if any) follows the shared fields as the review-specific extra line. Every P3 finding also includes the `fact_anchor` required by §5 — the `fact_anchor:` line is the P3 finding's `evidence:` form (it carries the comparison reference, the violating location, and the relationship), so a P3 finding does not repeat an `evidence:` block.

```
Findings:
  Batch group (N items) — fix does not change runtime behavior, suggested for batch handling:
    - [{P3}] {location} — {issue} (actionable)
      problem: {one-sentence statement naming the concrete subject}
      impact: {consequence if unaddressed}
      fix: {concrete repair action}
      fact_anchor: {reproducible repository fact and evidence locations}
    ...
  Decision group (M items) — decided one by one:
    [{severity}] {location} — {issue} (actionable | needs_decision)
      problem: ...
      evidence:
        - {verbatim quoted code} — {file:line}
      impact: ...
      fix: {repair action}   # actionable
      # or
      decision: {the question the user must answer}   # needs_decision
      options:
        - ...
      spec_context: {design context, if any}
      fact_anchor: {reproducible repository fact and evidence locations}   # required for P3, in place of evidence:
    ...
```

When batch grouping is inactive (threshold not met), the Findings section uses the flat format:

```
Findings:
  [{severity}] {location} — {issue} (actionable | needs_decision)
    problem: ...
    evidence:
      - {verbatim quoted code} — {file:line}   # P3 findings use the fact_anchor line instead
    impact: ...
    fix: ...   # or decision: ... with options: sub-lines
    spec_context: {design context, if any}
    fact_anchor: {reproducible repository fact and evidence locations, required for P3}
  ...
```

### Severity check

```
Severity check:
  confirmed: N | adjusted: N
  Severity confirmation: {finding_id} = confirmed {Px} — evidence: {file}; reason: {one line}
  Severity confirmation: {finding_id} = adjusted {Px} -> {Py} — evidence: {file}; reason: {one line}
  Severity confirmation: {finding_id} = confirmed {Py} — evidence: {file}; reason: {required final second check after an adjustment}
```

**PASS:** No P0 or P1 findings exist.
**FAIL:** One or more P0 or P1 findings exist — blocks promote.

**Counts:** the P0-P3 counts appear in the header's `Key counts`; the suppressed-by-spec count appears in the body's `Suppressed by spec (N)` block.

The gate outcome is carried by the unified skeleton's `Blocking promote: yes | no` header line; the Gate Rules table below decides it.

## Gate Rules

| Condition | Result |
|-----------|--------|
| P0 or P1 findings exist | FAIL — blocks promote |
| Only P2 or P3 findings | PASS — does not block promote |
| No findings | PASS — does not block promote |

For promote gate: a cache written by a full run with PASS result satisfies the promote requirement. Targeted re-reviews never write a cache, so they are informational only.

==ATOM_BEGIN:spec_review_standard==
# Spec Review Standard

## 1. Core Principle

`review` audits code quality. Its single difference from ordinary code review: for every potential finding, it checks the spec for a design rationale. If the spec explains why the code is written that way, the finding is suppressed.

It does NOT do:
- `validate` work (checking spec quality)
- `verify` work (checking spec-code alignment)

## 2. Pre-review Setup

Read the candidate spec (fall back to stable if no candidate exists) and extract design context:

| Context | Description | Typical Location |
|---------|-------------|------------------|
| `accepted_tradeoffs` | Design trade-offs the spec explicitly accepts | Design decisions section, rationale paragraphs |
| `architectural_decisions` | Conscious architectural choices | Spec body, architecture section |
| `design_constraints` | Design constraints | Constraints section, scope section |
| `known_debt` | Known technical debt | Known limitations, Future work section |
| `non_goals` | Explicitly excluded work | Non-goals section |

If no spec exists, run as ordinary code review without suppression.

## 3. Review Process

For each file in scope:
  1. Inspect for code quality issues
  2. For each potential finding:
     a. Check the spec for a design rationale:
        - Contained in `accepted_tradeoffs` → suppress
        - Contained in `architectural_decisions` → suppress
        - Required by `design_constraints` → suppress
        - Contained in `known_debt` → suppress, mark as tracked_debt
        - Covered by `non_goals` → suppress, mark as out_of_scope
        - No match → retain as finding
  3. Grade retained findings P0-P3
  4. For every finding graded P3, apply the P3 reportability gate below. Remove findings with no valid fact anchor; if the anchored impact exceeds P3, re-grade it under `framework/severity_policy.md` §9 instead of reporting it as P3.

Suppression does not alter severity — severity is code-only. Suppressed findings are simply not reported.

## 4. Review Dimensions

### Dimension 1: Structure & Boundaries

Module boundaries, responsibility separation, file structure.

- **What to flag**: Divergent Change (one file edited for unrelated reasons), Shotgun Surgery (one change scattered across files), Middle Man (excessive delegation)
- **Spec interaction**: Spec defines this as adapter/facade/aggregator → suppress structural findings
- **Not considered**: —

### Dimension 2: Naming & Abstraction

Whether names reveal intent, whether abstraction layers are appropriate.

- **What to flag**: Mysterious Name (name does not reveal purpose), Speculative Generality (abstraction for no current need)
- **Spec interaction**: None
- **Not considered**: —

### Dimension 3: Duplication

Repeated logic patterns.

- **What to flag**: Duplicated Code (same logic shape appears multiple times)
- **Spec interaction**: `known_debt` contains it → suppress
- **Not considered**: —

### Dimension 4: Coupling & Cohesion

Module coupling, module cohesion.

- **What to flag**: Feature Envy (method depends more on external object), Message Chains (long call chains), Refused Bequest (inherits unrelated behavior), Data Clumps (fields travelling together)
- **Spec interaction**: Spec designs this as tight coupling (e.g., adapter) → suppress
- **Not considered**: —

### Dimension 5: Error & Safety

Error handling consistency and safety.

- **What to flag**: Silently swallowed exceptions, broken error propagation paths, obvious null pointer risk, resource leaks, deadlocks
- **Spec interaction**: Spec defines delegated error handling → suppress
- **Not considered**: Spec-verify level requirement alignment

### Dimension 6: Hygiene

Dead code and unhealthy signals.

- **What to flag**: Uncalled functions, unused variables, commented-out code blocks, unreachable branches, outdated patterns inconsistent with the rest of the project
- **Spec interaction**: **None**. Dead code has no "intentional design" — flag on sight.
- **Not considered**: —

### Dimension 7: Unplanned Debt

Work traces not tracked by the spec.

- **What to flag**: TODO, FIXME, HACK, XXX markers
- **Spec interaction**: Contained in `known_debt` → suppress; not contained → flag
- **Not considered**: Spec-planned items are normal progress, not findings

### Dimension 8: Architectural Design Quality

Whether the implemented code structure forms an acceptable architecture for the unit's declared responsibility. This dimension evaluates the design surface of the code — module boundaries, responsibility organization, abstraction levels, dependency clarity, and extension landing points — as an overall architecture assessment, not as a smell checklist.

- **Assessment object**: The code under `implementation_surface` and `affects.files` — package structure, module boundaries, how responsibilities are organized, how components depend on each other, and how the structure matches the repository's established engineering patterns (layering, naming, error-handling conventions)
- **P0/P1 findings (gate-level, judged from code structure and spec declarations alone)**:
  - Spec-recorded architectural intent (e.g., declared layering, module boundaries) is implemented in a structure that has drifted beyond recognition → P0/P1
  - The unit reaches past the `unit_refs` boundary and reaches into a dependency unit's internal implementation → P1
  - Internal implementation organization is severely disconnected from the unit's declared responsibility (e.g., a "config loading" unit's surface contains business logic) → P1
- **P2 findings (advisory design-quality judgments)**: boundaries cut less naturally than the behavior domains suggest, abstraction levels slightly off, extension landing points less explicit than they could be
- **P3 findings**: only objective, local, low-impact inconsistencies or hygiene defects that pass the P3 reportability gate below; architectural, abstraction, and extension-shape preferences are not P3 findings
- **Spec interaction**: Spec-recorded architectural decisions with a conforming implementation are NOT re-questioned here — the recorded decision is authoritative (validated by `validate`). Assessment focuses on implementation drift from recorded intent and on code structure the spec does not cover
- **Not considered**: Design quality of spec-recorded decisions themselves (owned by `validate` Check 2); acceptance alignment (owned by `verify`)

## 5. Severity Levels

| Level | Definition | Characteristic | Example | Promote Gate |
|-------|-----------|----------------|---------|-------------|
| **P0** | Definitively causes production misbehavior | Determined from code structure alone, no runtime data needed | Null pointer dereference, deadlock, resource leak, race condition, incorrect lock usage, use-after-close | Block |
| **P1** | Inevitably causes maintenance pain or high-probability bugs | Latent but will surface over time | Silently swallowed exceptions, broken error propagation paths, large-scale logic duplication | Block |
| **P2** | Real but not severe | Affects readability and maintainability, not correctness | Mysterious Name, Feature Envy, localized Primitive Obsession, small Data Clumps | Don't block |
| **P3** | Objective, local style, clarity, or hygiene discrepancy | Reproducible from repository facts; does not affect correctness or materially harm maintainability | Unused import, minor established-convention deviation, stale comment with a direct code mismatch | Don't block |

P0/P1 findings block promote. The promote gate additionally requires a cache written by a full run — targeted keyword re-reviews do not write a cache, so they never satisfy the gate.

### P3 Reportability Gate

A finding may be reported as P3 only when all of the following are true:

1. It states a present discrepancy or absence, not a preference or a possible improvement.
2. It includes a `fact_anchor` that a second reviewer can reproduce from repository content. The anchor must identify the governing or comparison reference, the violating location, and the relationship between them. A location reference by itself is not a fact anchor.
3. The fact anchor uses one of these proof shapes:
   - **Absence**: a declaration has no caller, reader, or reference within a defined repository scope.
   - **Contradiction**: a specification, comment, or interface promise conflicts with the implementation.
   - **Convention deviation**: an explicit repository rule or a repeated same-family pattern is violated at the reported location.
4. The impact is local and low: it does not affect correctness and does not materially harm maintainability. A material maintainability impact is P2 or higher under `framework/severity_policy.md` §9.3; an impact that cannot be established is not a reportable P3.
5. The recommendation is either one concrete repair or a clearly identified decision item. A recommendation containing "consider", "or", or "maybe" is not eligible for the batch group; those words do not by themselves invalidate a real P3 decision item.

Dead code, comment/code contradiction, established-convention deviation, and an unreachable comment or interface promise are common examples of these proof shapes, not an exhaustive issue whitelist. If the anchor is missing or fails to establish a present discrepancy, remove the finding rather than demoting it to P2.

## 6. Finding Output Format

```
[{severity}] {location} — {issue} (actionable | needs_decision)
  problem: {one-sentence statement naming the concrete subject}
  evidence:
    - {verbatim quoted code} — {file:line}
  impact: {consequence if unaddressed}
  fix: {concrete repair action}   # actionable
  # or
  decision: {the question the user must answer}   # needs_decision
  options:
    - {candidate option}
  spec_context: {relevant design context from spec, if any}
  ref: {optional tracking anchor}
```

Each finding contains:
- `severity`: P0-P3
- `location`: file path + line number
- `issue`: description of the problem
- `problem`: one sentence naming the concrete subject and what is wrong with it
- `evidence`: the quoted code that proves the claim; a P3 finding uses its `fact_anchor` line as the evidence form instead
- `impact`: what goes wrong, or stays undecidable, if the finding is not resolved
- `fix` (actionable) / `decision` with `options` (needs_decision): the concrete repair action or the decision the user must make
- `spec_context`: (optional) relevant design context from the spec, helps the user understand the code-design relationship
- `fact_anchor`: (required for P3) the reproducible repository fact, comparison or governing reference, violating location, and relationship that proves the P3 discrepancy
- `ref`: (optional) anchor or line reference for tracking only — it carries no meaning the rest of the finding does not already state

**Dependency scope report:** In addition to findings, every sub-agent reports the read scope of its packet — for the reviewed file, the section-region headings (or 1-based closed line ranges; `all` when the assessment covered the whole file) its review judgment actually depended on:

```
Dependency scope:
  {check key}: {file}: {declaration}   # check key = the reviewed file path; declaration = section heading, line ranges, or "all"
```

Review judgments commonly cover whole files (a code quality assessment has no partial scope) — report `all` honestly in that case; the cache's `deps` then covers the whole file by design. The packet report declares the scope; `gate-submit` validates it against that packet's `read_refs` (not merely the run-wide snapshot), and `gate-finalize` computes the CIDs and records the per-check breakdown (check key = the reviewed file path) in the cache's `checks` mapping — section headings become section-region dependencies for the unit's own main spec, line ranges become chunk declarations (see `framework/validation_cache.md` §Format → Per-check evidence); the declared ranges must cover every region the review judgment depended on, including called functions and referenced structures.
==ATOM_END:spec_review_standard==

### Stable-only targets

When no candidate exists, the review runs with the stable spec as design context (see §8 Write review cache — the stable review writes `target: stable`, a quality confirmation state consumed by `fresh@stable`; it grants no promote eligibility).

## 7. Cross-Check

### 7.1 Purpose

After collection, before output — validate that each finding holds in the full codebase and full document set, not just in the local review context.

Every review run needs this:
- **Full** — distributes work to sub-agents; each sees only its slice, not cross-unit implementation details.

### 7.2 Process

1. **Cross-file authenticity check** — For each finding whose issue asserts "X lacks Y capability" or "parameter Z may not be handled", read the callee/target implementation. If the target handles the concern (nil guard, empty-string path, idempotent close, etc.), the finding is a false positive.

2. **Cross-document consistency check** — For each finding that relies exclusively on the spec main body, check appendices and related documents for supplementary or overriding references. If the full document set resolves the issue, the finding is invalid.

3. **P3 fact-anchor check** — For every P3 finding, the cross executor reads each cited anchor and reproduces the claimed relationship. If the anchor is missing, does not establish a present discrepancy, or relies only on reviewer preference, disposition the finding as `suppressed`. Do not demote an unsupported P3 to P2.

4. **Remove false positives** — Findings that dissolve under cross-check receive an explicit `suppressed` disposition with evidence and are excluded from the generated user report and counts. The disposition remains in the audit artifact.

### 7.3 Complexity

Lightweight: one independent cross packet reads the necessary target files and every accepted per-file result. It does not relaunch file reviewers or repeat the full review.

### 7.4 Severity Consistency Check

As part of synthesis, the cross executor confirms each retained finding's severity before its result can be accepted, per `framework/severity_policy.md` §9. The main agent does not re-grade findings after cross acceptance.

For every retained finding (P0-P3):

1. Read at least one target file beyond the reviewed surface that the finding's impact claim depends on (caller, callee, consumer, or the spec document governing the affected behavior)
2. Verify the severity boundary from `severity_policy.md` §9.3 holds against the read evidence
3. Record one complete result sequence per finding with the exact `Severity confirmation:` syntax from `framework/verification_scope.md`:
   - `confirmed {Px}` — severity stays and the sequence is complete
   - `adjusted {Px} -> {Py}` — with the evidence file read and a one-line reason, followed by exactly one final `confirmed {Py}` or adjacent `adjusted {Py} -> {Pz}` record
4. Publish the adjusted severity in the cross result; `gate-finalize` computes the gate result from the retained P0/P1 findings

Evidence rules: upgrade requires positive evidence read from the target; downgrade requires completed reading that confirms the protective path or contained impact; no evidence → keep the original severity; one level per adjustment, at most two iterations (§9.4). The severity records appear in the output's Severity check block (§Output Format) so the review trace shows the check ran.

---

## Batch classification (review)

When findings exist, the main agent classifies each finding into a **batch group** or a **decision group** before presentation. Classification runs after the cross-check (§7) — cross-check has already removed false positives; classification aggregates each finding's severity and recommendation, and for batch group candidates only runs lightweight assertion re-verification (see Assertion re-verification below).

**Activation threshold:** Batch grouping is enabled only when the total finding count ≥ 10 AND the batch group would contain ≥ 3 items. Below the threshold, present findings flat (§Output Format without grouping).

**Batch group eligibility (ALL must hold):**
- Severity is P3 — P0/P1 are gate-level, P2 is a trade-off decision, all go to the decision group
- Fix type ∈ {remove unused import / unused variable / dead code / commented-out block, update stale comment} — the fix does not change any runtime behavior
- Recommendation is a single concrete action — no "or", "consider", "maybe" wording
- Involves ≤ 3 files, all within the same change domain

All other findings — P0/P1/P2, any refactoring-type fix (structure, duplication, coupling, naming), error/safety findings, TODO/debt markers — go to the decision group.

==ATOM_BEGIN:batch_findings_mechanism==
**Assertion re-verification:** After eligibility passes, the main agent re-verifies each batch group candidate's core assertion with 1-2 deterministic checks:
- Sub-agent claims "X is missing" → confirm X is really absent from the cited location (read the file, check existence, or grep as appropriate)
- Sub-agent claims "the cited source states X explicitly" → read the cited line, confirm X is really written there, without vague wording ("or", "suggested", alternatives)
- Sub-agent claims "the correct pattern exists elsewhere" → confirm that reference really exists

Re-verification failure → item moves to the decision group. This runs before execution because a post-execution check re-run cannot detect a wrong-direction fix — once the documents are made consistent, the re-run passes and the error is cemented.

**Execution boundary:** Classification is presentation only — it is not an authorization to act. Nothing is implemented until the user explicitly agrees to the whole batch group. The user may approve the batch, release it without changes, or move individual items out to the decision group. Before confirming, the user may ask to expand any batch item (full analysis plus on-the-spot re-check); expanded items move to the decision group and are decided individually.

**Batch group presentation note:** When batch grouping is active, add: "This grouping is a classification suggestion only — nothing is applied until you confirm. Each item shows its judgment basis; ask to expand any item if in doubt — expanded items move to the decision group."
==ATOM_END:batch_findings_mechanism==

After classification, present the findings (§Output Format) and wait for the user's decision per HARD RULE 3a:
- **Batch group:** one decision on the whole group per §Batch classification.
- **Decision group:** present each finding with its severity and recommendation and wait for the user's decision per HARD RULE 3a. Do not offer a structured resolution menu.

---

## Post-fix Spec Update Obligation

Fixing a review finding does not automatically create a spec update obligation. The only trigger: the fix changes a behavior the spec declares or implicitly promises — the spec says behavior X holds, the fix makes it Y, and the spec becomes inaccurate. Directional basis: this is the boundary definition of HARD RULE 1's "Create or update the candidate spec only for requested design changes" (`framework/concepts.md`).

**Judgment basis:** whether the spec declares or is silent on a behavior is judged by the contract statement definition and its external-visibility boundary in `framework/spec_writing_guide.md` §4: a declared externally-observable behavior is a promise; internal implementation detail is design expression and creates no promise.

| Fix effect on the spec | Spec update obligation |
|------------------------|------------------------|
| Fix changes a behavior the spec declares or implicitly promises | **Must update** (e.g. RegisterAll pointer zero-value: the spec's fixed semantics implicitly promise "RegisterAll always retained"; the fix changes it to clearing) |
| Spec is silent on the behavior — it never promised anything | **No update**; the spec stays accurate. Recording the behavior is an optional design-decision note, not an obligation (e.g. consent out-of-range fail-closed default: worth anchoring, but not required by this criterion) |
| Fix touches internal details (dead code, naming, comments, tests, encapsulation) | **Never update** |

Recording a safety-critical behavior where the spec is silent is advisory, not mandatory — observable behavior must not be automatically elevated to a "must write" obligation. The spec records design decisions and contract boundaries; unspecified behavior is not a spec gap by itself.

Stable-only targets: a triggered spec update goes through `specflowctl fork --unit <name>` — the stable spec is never edited directly (see §8 Stable-only target).

## 8. Write review cache (tooling)

After the cross result is accepted, presentation-only batch classification may run. It cannot change the accepted result. Then:

1. `gate-finalize` computes the gate result:
   - P0 or P1 findings exist → `result: fail`, `blocking: true`
   - Otherwise → `result: pass`, `blocking: false`

2. `gate-finalize` writes `docs/specs/meta/validation/unit/{name}/review_result.md` per `framework/validation_cache.md` format:
   - Create `docs/specs/meta/validation/unit/{name}/` directory if needed
   - Include `mode: full`, `target: candidate`, severity counts, `blocking`, file hashes and dependency CIDs
   - Include full findings body (cannot be omitted — required for promote gate detail)
   - On FAIL, include the complete per-file `status` map from the accepted cross synthesis (`pass`/`fail`; full runs have no `carried`). This is the failure-recovery scope input

Full runs always write cache regardless of pass/fail. Targeted runs (`:{keyword}`) never write a cache, and a targeted run that finds P0/P1 deletes a pass cache (a blocking cache is kept — it is already blocking and is the recovery baseline) — blocking findings at any granularity mean promote must not proceed. See `framework/validation_cache.md`.

**Stable-only target:** write the review cache with `target: stable` — the quality confirmation state consumed by `fresh@stable` (same shape: `mode: full`, severity counts, `blocking`, file hashes and dependency CIDs, findings body). It grants no promote eligibility; delta re-runs (`rereview`) apply to stable-only targets with a usable baseline — a pass cache (STALE recovery) or a blocking cache (failure-record recovery, §Failure recovery); a MISSING stable cache needs the full confirmation run (see `framework/verification_scope.md` §Delta Runs → Layer applicability). Finding handling for a stable review: implementation-class defects (code-local issues) may be fixed in code and re-reviewed; design-class defects (root cause in the stable spec's intent) lead to forking (`specflowctl fork --unit <name>`) — the stable spec itself is never edited directly.

**Cache record contract:** The cache records the file state read during the review run — it is independent of the user's decision outcome:

==ATOM_BEGIN:cache_evidence_path_forms==
**Declaring cache evidence — path forms:** spec objects resolved by name other than the run's own target files are declared as **logical references** instead of physical paths — `unit:{name}` for a unit main spec, `unit:{name}:appendix:{file}` for a unit protocol appendix (the full appendix file base name without `.md`, e.g. `unit:auth:appendix:unit_auth_account_token_claims`), `rule:{id}` for a rule file — with the `hash` + `deps` of the file actually read. The run's own target files (the unit's own main spec and appendices; for a rule target, the candidate rule file and its stable sibling) and code files keep physical paths. A logical reference resolves at freshness time to the current-layer file (candidate first, stable fallback), so promoting the referenced unit or rule does not stale a cache whose dependency content is unchanged (see `framework/validation_cache.md` §Logical References).
==ATOM_END:cache_evidence_path_forms==

- Each packet report declares the files it read in its `Dependency scope:` lines (`ranges` / `sections` / `acceptance_items`); `gate-submit` validates path membership against that packet's `read_refs` (not merely the run snapshot), and `gate-finalize` computes the `hash` + `deps` evidence from the accepted reports. The values come from the sub-agents' `Dependency scope` reports (see §6); a file reported as `all` (or not reported) is declared as a whole file. The declared ranges must cover every region the review judgment depended on — including called functions and referenced structures (declare-heavy principle; see `framework/validation_cache.md` §Dependency Declaration). **The unit's own main spec (design context) is declared by section regions per reviewed file:** the packet reports' `sections` values become the cache entry's `checks` mapping (check key = the reviewed file path), with the union in `deps` (see `framework/validation_cache.md` §Format → Per-check evidence). Code files keep chunk-CID declarations; whole-file (`all`) declarations carry no per-check breakdown
- Write the cache before applying any user-approved fixes — presenting findings and waiting for the user's decision is presentation only and does not gate the cache write
- Any fix applied after the cache write makes the cache stale (promote's dependency check fails) — the review gate must be recovered before promote. This is a promote-gate requirement enforced when the user triggers promote; it does not authorize automatic re-review after fixes. The agent must not re-run review on its own initiative (see HARD RULE 2 in `framework/concepts.md`). After a fix, the agent guides the user to a targeted re-review (`review@{unit}:{keyword}`), a delta re-run (`rereview@{unit}` — from a stale pass cache or, after resolving its findings, from a blocking cache, §Delta re-run), or a concrete full command and waits for the user to trigger it; the full re-run is triggered by the user when deciding to promote.

### Delta re-run (rereview@{unit})

Candidate targets, plus stable-only targets with a usable baseline — a delta re-run against a stable-only target whose confirmation cache is MISSING reports the missing-baseline message — "No usable confirmation baseline. Run the full `{command}@{target}` (confirmation check) first, or `specflowctl fork {--unit|--rule} {name}` to start a new round." — and stops (see `framework/verification_scope.md` §Delta Runs → Layer applicability). Triggered by the user when the cache is STALE or BLOCKED (a blocking cache — the failure record). Follow §Delta Runs in `framework/verification_scope.md`:

1. **Preconditions** — the existing cache must have `mode: full` and one of the two baselines: `blocking: false` (STALE recovery) or `blocking: true` (failure-record recovery, §Failure recovery). A MISSING cache has no baseline — run `review@{unit}` instead.
2. **Plan (mechanism-derived)** — run `specflowctl gate-plan --gate review --unit {name} --target {candidate|stable} --mode delta` (or `--mode repair` for a blocking cache). The planner derives the re-run set from the cache's per-check `checks` mapping — affected = {files whose declared deps went stale} ∪ {current files the baseline never declared} ∪ {persisted `invalidated_checks`} ∪ {explicit `--rerun` overrides} ∪ {cross-check} — and degrades conservatively to the full packet set when a stale source has no per-check mapping, an invalidated key cannot map to the current review surface, or the cache lacks per-check evidence. After a targeted P0/P1, the coordinator runs `gate-invalidate --check {file path}` immediately; repair reads that state automatically and never carries the contradicted file. It reports the re-run file packets, carried-over files (`carried_keys`), and any degradation or full-scope coverage statement. The plan is the scope; the executor does not re-derive or extend it.
3. **Execution** — execute the planned packets (protocol: `framework/verification_scope.md` §Gate Work Packets), submitting each report with `specflowctl gate-submit --run <run_id> --packet <packet_id> --report PATH`. Any P0/P1 finding → the finalize writes the review failure record (`result: fail`, `blocking: true`, severity counts, findings body, `basis: delta`, per-file `status` map — `fail`/`pass` for the re-run files, `carried` for the carried-over ones, derived by `gate-finalize` from the accepted packet verdicts); present findings, stop.
4. **On PASS** — `gate-finalize` rewrites `review_result.md` with `result: pass`, `mode: full`, `basis: delta` (or `basis: repair` when recovering from a blocking cache), `target: candidate` (stable-only target: `target: stable`), `blocking: false`, severity counts, a fresh `timestamp`, a **complete** `files` list (new `hash` + `deps` + per-check `checks` evidence computed by the tooling from the accepted packet reports for re-reviewed files, original evidence merged by the tooling from the baseline cache for carried-over files), and the findings body. Logical references (`unit:{name}` / `unit:{name}:appendix:{file}` / `rule:{id}`) are carried over the same way.

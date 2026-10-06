# Verify Checklist (both lenses)

## Overview

`verify` is one gate with **two lenses** that run in separate sessions:

- **`alignment`** — the spec-vs-code check: acceptance items, structural alignment, scope, retirement, surplus, stubs, and divergence analysis (Steps 1–7 below). The spec is **authority**: a divergence is always a finding and is never suppressed.
- **`quality`** — the spec-aware code-quality check: structure/boundaries, naming/abstraction, duplication, coupling, error & safety, hygiene, unplanned debt, architectural quality, and test quality. The spec is **rationale**: a code-quality finding is suppressed when the spec records the design decision behind it.

The trigger route in `framework/concepts.md` loads this file at verify time, not proactively. The judge of every key is an independent read-only subagent; the main agent partitions the run's coverage set into lens-pure sessions, generates each session's mission with `gate-mission`, and records each report with `gate-submit` (see `framework/verification_scope.md` §Coverage model).

## Lenses and boundary ownership

A session's keys come from **one lens only** — a session never mixes lenses. The two lenses share one P0–P3 severity scale and are merged into the one `verify_result.md` cache, which carries an `alignment` section and a `quality` section.

Coverage keys:

| Lens | Coverage keys |
|---|---|
| `alignment` | `item:<unit>:<item>` and `preserve:<unit>:<item>` |
| `quality` | `code:<file>`, `design:<unit>:<file>`, and one `architecture:<unit>` |

**Boundary ownership** — which lens owns which judgment:

| Judgment | Owner | Rule |
|---|---|---|
| Test **quality** (assertion authenticity, mock density, tautology, happy-path-only) | `quality` | Tests are graded as code here. The `alignment` lens uses tests only as behavioral evidence for an item, never as a quality grade. |
| Code within this unit's responsibility with **no spec basis** (surplus) | `alignment` **first** | The `alignment` lens attributes responsibility before judging surplus. Only if the code survives that decision — the code is legitimate and the spec gap is not a drift — does it become a `quality` judgment. |
| **Stubs** | by nature | A stub that violates a *declared* behavior is an `alignment` finding (implemented-but-absent). A stub that is merely poor code (unreachable, placeholder without a declared behavior) is a `quality` finding. |
| **Debt markers** (TODO/FIXME/XXX/HACK comment markers) | `quality` | Dimension 7 owns unplanned-debt markers: a comment marker is a work trace, not a behavioral mismatch. A marker contained in the spec's `known_debt` is suppressed; an undeclared marker is a non-blocking P2/P3 observation. Stubs and placeholders (missing implementation where behavior is declared) remain `alignment` Step 6 findings. |
| **Spec-sanctioned suppression** | `quality` **only** | A recorded spec rationale suppresses a `quality` finding. The `alignment` lens never suppresses a divergence. |
| Structural alignment, scope, retirement, acceptance, divergence analysis (Steps 1–7) | `alignment` | — |

## The quality lens

The `quality` lens's standard — core principle, pre-review setup, review process, the eight dimensions, the P3 reportability gate, the finding output format, and the dependency-scope report — is defined in full below and is delivered into this file by the atom `spec_review_standard`.

==ATOM_BEGIN:spec_review_standard==
## Quality Lens Standard

The `quality` lens separates reusable public code facts from unit-specific design judgments and the unit's overall architecture assessment. The complete protocol is `framework/shared_judgments.md`.

### 1. Core Principle

`code:<file>` records facts and potential problems without a unit-private rationale. `design:<unit>:<file>` treats that unit's spec as rationale: actively check its requirements and retain or exclude every public observation from its assigned file's immutable record with evidence. Public execution batches do not enlarge the design session's inputs. A unit-specific exclusion never removes a public observation. A co-batched session holds a file's `code:<file>` key and its unit's `design:<unit>:<file>` key together: the same reviewer collects the facts and then judges the unit design against them in one pass — the facts stay rationale-free and publish as the file's public record, so other units can still reuse them. `architecture:<unit>` assesses Dimension 8 once for the entire unit. The alignment lens treats the spec as authority; protected stable requirements must be ALIGNED, or an evidence-backed record-drift routing.

### 2. Pre-review Setup

Public checks read the complete file evidence surface fixed by the mission, including related callers, callees, dependencies, tests and applicable public rules. Do not read unfinished peer designs. A missing evidence path requires replanning.

Design and architecture checks read their selected unit spec, applicable rules and code. Extract accepted trade-offs, architectural decisions, design constraints, known debt and non-goals from that unit's published design context.

### 3. Review Process

1. Reuse accepted public observations only when coverage, code content and protocol are valid; otherwise execute the public check.
2. Consume the public records for the design session's assigned files only, whether executed, reused or carried. For each observation in those records, the design reviewer reports `Observation disposition: <id> = retained|suppressed — <unit-specific evidence and reason>`. In a co-batched session the design reviewer disposes the observations collected in the same report's code block instead of consuming a published record: write the code block first, then reference each observation by its tool-assigned id — the k-th potential finding in the code block is `<run>/<session>/F<k>`.
3. Actively check the unit's spec requirements, including violations not present in the public record. Report new findings independently.
4. Assess the six Dimension 8 fields in the unit's architecture task once.
5. Public potential problems are observations rather than gate-driving findings. Unit design findings drive this unit's gate. The standard severity and finding format below applies to their presentation.

### 4. Review Dimensions

#### Dimension 1: Structure & Boundaries

Module boundaries, responsibility separation, file structure.

- **What to flag**: Divergent Change (one file edited for unrelated reasons), Shotgun Surgery (one change scattered across files), Middle Man (excessive delegation)
- **Spec interaction**: Spec defines this as adapter/facade/aggregator → suppress structural findings
- **Not considered**: —

#### Dimension 2: Naming & Abstraction

Whether names reveal intent, whether abstraction layers are appropriate.

- **What to flag**: Mysterious Name (name does not reveal purpose), Speculative Generality (abstraction for no current need)
- **Spec interaction**: None
- **Not considered**: —

#### Dimension 3: Duplication

Repeated logic patterns.

- **What to flag**: Duplicated Code (same logic shape appears multiple times)
- **Spec interaction**: `known_debt` contains it → suppress
- **Not considered**: —

#### Dimension 4: Coupling & Cohesion

Module coupling, module cohesion.

- **What to flag**: Feature Envy (method depends more on external object), Message Chains (long call chains), Refused Bequest (inherits unrelated behavior), Data Clumps (fields travelling together)
- **Spec interaction**: Spec designs this as tight coupling (e.g., adapter) → suppress
- **Not considered**: —

#### Dimension 5: Error & Safety

Error handling consistency and safety.

- **What to flag**: Silently swallowed exceptions, broken error propagation paths, obvious null pointer risk, resource leaks, deadlocks
- **Spec interaction**: Spec defines delegated error handling → suppress
- **Not considered**: Alignment-level requirement alignment (owned by the `alignment` lens)

#### Dimension 6: Hygiene

Dead code and unhealthy signals.

- **What to flag**: Uncalled functions, unused variables, commented-out code blocks, unreachable branches, outdated patterns inconsistent with the rest of the project
- **Spec interaction**: **None**. Dead code has no "intentional design" — flag on sight.
- **Not considered**: —

#### Dimension 7: Unplanned Debt

Work traces not tracked by the spec.

- **What to flag**: TODO, FIXME, HACK, XXX markers
- **Spec interaction**: Contained in `known_debt` → suppress; not contained → flag
- **Not considered**: Spec-planned items are normal progress, not findings

#### Dimension 8: Architectural Design Quality

Whether the implemented code structure forms an acceptable architecture for the unit's declared responsibility. This dimension evaluates the design surface of the code — module boundaries, responsibility organization, abstraction levels, dependency clarity, and extension landing points — as an overall architecture assessment, not as a smell checklist.

- **Assessment object**: The code under `implementation_surface` and `affects.files` — package structure, module boundaries, how responsibilities are organized, how components depend on each other, and how the structure matches the repository's established engineering patterns (layering, naming, error-handling conventions)
- **P0/P1 findings (gate-level, judged from code structure and spec declarations alone)**:
  - Spec-recorded architectural intent (e.g., declared layering, module boundaries) is implemented in a structure that has drifted beyond recognition → P0/P1
  - The unit reaches past the `unit_refs` boundary and reaches into a dependency unit's internal implementation → P1
  - Internal implementation organization is severely disconnected from the unit's declared responsibility (e.g., a "config loading" unit's surface contains business logic) → P1
- **P2 findings (advisory design-quality judgments)**: boundaries cut less naturally than the behavior domains suggest, abstraction levels slightly off, extension landing points less explicit than they could be
- **P3 findings**: only objective, local, low-impact inconsistencies or hygiene defects that pass the P3 reportability gate below; architectural, abstraction, and extension-shape preferences are not P3 findings
- **Spec interaction**: Spec-recorded architectural decisions with a conforming implementation are NOT re-questioned here — the recorded decision is authoritative (validated by `validate`). Assessment focuses on implementation drift from recorded intent and on code structure the spec does not cover
- **Not considered**: Design quality of spec-recorded decisions themselves (owned by `validate` Check 2); behavioral alignment (owned by the `alignment` lens)

### 5. Test Quality (owned by this lens)

Test code is judged by the `quality` lens. The `alignment` lens considers tests only as behavioral evidence for an acceptance item; it does not grade their quality. This lens grades test quality along the established dimensions: are the tests behavioral or implementation-coupled, do they assert meaningful outcomes, are failure modes covered, is test setup proportionate, and does the suite avoid brittle duplication. A test-quality finding is suppressible when the spec records the testing decision that produced it.

### 6. Severity Levels

| Level | Definition | Characteristic | Example | Promote Gate |
|-------|-----------|----------------|---------|-------------|
| **P0** | Definitively causes production misbehavior | Determined from code structure alone, no runtime data needed | Null pointer dereference, deadlock, resource leak, race condition, incorrect lock usage, use-after-close | Block |
| **P1** | Inevitably causes maintenance pain or high-probability bugs | Latent but will surface over time | Silently swallowed exceptions, broken error propagation paths, large-scale logic duplication | Block |
| **P2** | Real but not severe | Affects readability and maintainability, not correctness | Mysterious Name, Feature Envy, localized Primitive Obsession, small Data Clumps | Don't block |
| **P3** | Objective, local style, clarity, or hygiene discrepancy | Reproducible from repository facts; does not affect correctness or materially harm maintainability | Unused import, minor established-convention deviation, stale comment with a direct code mismatch | Don't block |

P0/P1 findings block promote. The promote gate additionally requires a cache written by a full run — targeted re-checks do not write a cache, so they never satisfy the gate. The `quality` lens and the `alignment` lens share the same P0–P3 scale; their graded findings are merged into the one `verify_result.md` cache.

#### P3 Reportability Gate

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

### 7. Finding Output Format

Public and design sessions contain exactly one `File: {assigned check key}` block per assigned file. Public blocks contain `conclusion: FACTS`, `facts`, potential observations and whole-file scopes for every public input. Design blocks contain a quality conclusion, `spec_requirements`, `gate_findings`, an evidence-backed disposition for each public observation, new findings and dependency scopes. Architecture uses one `Unit: architecture:{unit}` block, the six Dimension 8 assessments, conclusion, `gate_findings`, `Suppressed by spec (N)` and dependency scopes. Batching never shares one subject's verdict or findings with another.

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

**Dependency scope report:** In addition to findings, every sub-agent reports the read scope of its session — for the reviewed file, the section-region headings (or 1-based closed line ranges; `all` when the assessment covered the whole file) its review judgment actually depended on:

```
Dependency scope:
  {check key}: {file}: {declaration}   # check key = code:<file>, design:<unit>:<file>, or architecture:<unit>; code uses all
```

Every code input uses an exact whole-file fingerprint, including evidence read by a design, architecture or acceptance judgment. Public checks declare `all` for every input. Spec dependencies may use chapters or acceptance-item regions. The session report declares the scope; `gate-submit` validates it against that session's `read_refs` (not merely the run-wide snapshot), and `gate-finalize` computes the CIDs and records the per-check breakdown (check key = the assigned task key, lens = `quality`) in the cache's `checks` mapping — section headings become section-region dependencies for the unit's own main spec, line ranges become chunk declarations (see `framework/validation_cache.md` §Format → Per-check evidence); the declared ranges must cover every region the review judgment depended on, including called functions and referenced structures.
==ATOM_END:spec_review_standard==

## The alignment lens

The `alignment` lens is the remainder of this file (Steps 1–7 and the analysis collection). It treats the spec as authority and never suppresses a divergence.

## Prerequisite — Read all unit files

Before executing any verify step, the agent must read the complete unit spec:

1. Read the main spec: `docs/specs/units/candidate/unit_{unit}.md`
2. Glob all candidate appendix files: `docs/specs/units/candidate/appendix/unit_{unit}_*.md`
3. For each appendix, read its frontmatter. Skip exempt content and confirm ownership by the appendix `unit` field.
4. Read the content of every non-exempt appendix file.

The unit's complete spec is the union of the main spec and all non-exempt appendix files. All verify steps that follow operate on this union — appendix content (API contracts, data types, error codes, state machines) is part of the spec and must be verified against code.

## Mode Selection

| Trigger | Mode | What to execute |
|---------|------|-----------------|
| `verify@{unit}` | full | Verify all spec content and the declared code surface: the `alignment` lens (Steps 1–7 per acceptance item) and the `quality` lens (public code and unit design per file, plus one unit architecture task), and protection of related stable requirements. Sessions share the same kind and lens. |
| `verify@{unit}:{keyword}` | targeted | Match keyword to spec content by title, feature name, API path, or structure → verify that content. Does not write a cache. |

**Keyword domain:** verify keywords resolve to spec content — section titles, feature names, API paths, acceptance item ids, appendix files. A keyword matching no spec content is a no-match — ask the user for clarification.

**Output:** Targeted runs report only the requested content and note "This was a targeted check — no complete cache was written. Run `verify@{unit}` for a complete verification." A targeted P0/P1 must first be persisted with `gate-invalidate --check {item id}`.

**Cache:** see `framework/validation_cache.md` for format.

## Core Principle

Verify is a **static structural alignment check** — it compares what the spec describes against what the code implements, using static file inspection (reading file content, searching text by pattern, locating files by name pattern) and read-only repository history queries. It cannot run code, call APIs, or capture runtime output. The "evidence" in verify is **code references**: file paths, line numbers, and code snippets proving structural existence and consistency.

**Adversarial stance:** verification starts from the assumption that the spec is NOT aligned with the code. Every ALIGNED claim must cite a deterministic check (grep, file existence, line count) — "I read the code and it looks correct" is not sufficient evidence for ALIGNED. Claims without deterministic evidence must be reported as CANNOT_DETERMINE.

**Verdict folding:** an item's verdict is decided claim by claim:

| Evidence state | Verdict |
|---|---|
| Every normative claim has deterministic evidence, no counterexample | ALIGNED |
| At least one claim has a confirmed counterexample | MISMATCH (affected claims listed) |
| No counterexample, but ≥1 claim cannot be proven statically | CANNOT_DETERMINE (gap listed) |
| Test absence / Part A-B concerns | No verdict change — recorded as annotations |

Fold order: MISMATCH > CANNOT_DETERMINE > ALIGNED. A claim supported only by the existence of a test is not deterministic evidence — tests are annotations, not proof of implementation behavior.

After completing all analysis steps, the agent must report coverage confidence (see §Output Format Coverage).

## Target Selection

- If a candidate spec exists → verify code against **candidate** (the current working proposal). Mismatches trigger first-principles divergence analysis (Step 7).
- If no candidate exists but a stable spec does → verify code against **stable** (check if current implementation still conforms to recorded truth). Do not enter divergence resolution — instead, recommend forking the unit (`specflowctl fork --unit <name>`).

## Execution Rules

- **Subagent permissions:** may inspect file content, search text by pattern, locate files by name pattern, and query read-only repository history (e.g., `git log` for file timestamps). Must NOT modify files or execute commands that change state. The main agent launches one read-only sub-agent per session (each session covers an agent-chosen, lens-pure batch of coverage keys) — each sub-agent follows the same permissions (read-only, no delegation chain).
- Each verifiable claim in the spec reports **ALIGNED** / **MISMATCH** / **CANNOT_DETERMINE** with code references.
- Evidence is always code-level (file:line, struct/function signatures, grep results) — never runtime output.
- A symbol name, comment, test function name, or interface existence alone is not evidence of behavior — read the code that implements the behavior (e.g. an enum value is confirmed by its definition and JSON tag, not by its identifier).
- For CANNOT_DETERMINE claims (e.g., pass_condition requires runtime verification): record the gap and continue.

## Output Format

==ATOM_BEGIN:report_skeleton==
## Unified Report Skeleton

All quality-gate reports (validate and verify) share the same report skeleton below. The header lines (`Result`, `Blocking promote`, `Key counts`), the Findings block fields, and the `Next step` line are identical across gates; only the body content and the gate-specific extra lines inside each finding block are gate-specific and defined in each checklist file. The Findings section follows the header because on FAIL it is the decision surface — the body and Dependency scope sections are its audit and evidence.

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
Dependency scope:
  {check key}: {file}: {declaration}   # per executed check; declaration = section heading text, acceptance item id list (acceptance_item:<id>), the whole-set token acceptance_items, line ranges, or "all"
────────────────────────────────────────────
Next step: {concrete next command with reason, or "None"}
────────────────────────────────────────────
```

**Field definitions:**

- `{gate}@{target}` — the gate and target that produced this report, e.g. `validate@user_auth`, `verify@user_auth`. Gates: `validate`, `verify`. Targets: unit or rule name. The `verify` gate carries two lenses — `alignment` (spec-vs-code) and `quality` (spec-aware code quality) — and its sessions report under one of them.
- `{mode}` — `full` for full runs; `targeted (user requested: {keyword})` for targeted runs; `delta` for incremental re-runs (`revalidate@{target}` / `reverify@{unit}`).
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
- `Dependency scope:` — one line per check the run executed: `{check key}: {file}: {declaration}`. `{check key}` is the gate's check identifier (validate: `check-{n}`; verify `alignment`: `item:<unit>:<item>` or `preserve:<unit>:<item>`; verify `quality`: `code:<file>`, `design:<unit>:<file>` or `architecture:<unit>`). `{declaration}` is the section-region heading text the check's judgment read (e.g. `Description`, `Testability / Acceptance Criteria`; the frontmatter region is `frontmatter`), `acceptance_item:<id>[,<id>...]` (one or more acceptance item regions of the spec's `acceptance_item_set` — the precise declaration for a judgment over specific items, e.g. one verify alignment judgment), the reserved token `acceptance_items` (the whole `acceptance_item_set` structural region — for judgments over the set as a whole, e.g. validate's acceptance coverage check), 1-based closed line ranges (e.g. `120-180,300-320`), or `all` when the judgment covered the whole file. Every read-only subagent reports this scope for the checks in its session; the report is submitted verbatim via `gate-submit`, which validates the declarations against that session's `read_refs` — not merely the run-wide snapshot — (path membership and declaration parseability), and `gate-finalize` computes the CIDs and records the per-check breakdown, tagged with the session's lens, in the cache's `checks` mapping (see `framework/validation_cache.md` §Format → Per-check evidence). Delta/repair runs report the scope of the re-run checks only — carried-over checks are not re-executed and get no new declaration (see `framework/verification_scope.md` §Sub-agent Prompt Assembly → Check / session scope). Targeted runs may omit it.
- `Incremental scope:` — delta runs only (mode `delta`). One line per re-run coverage key in the run's own structure (e.g. validate: "structural (checks 1, 3, 6): re-run — section `Description` of the unit's own spec changed"), followed by a line declaring the carried-over checks ("checks 1-6, 8-10: carried over — their dependency evidence is unchanged"). The scope is mechanism-derived at plan time from the cache's per-check evidence: `gate-plan` maps stale regions to the checks that declared them, adds the current keys the baseline never declared (a new acceptance item or code file has no evidence to carry, so it executes like a stale judgment), and fixes the re-run coverage set (see `framework/verification_scope.md` §Delta Runs). For a failure-record recovery (`basis: repair` — the run recovers a delta FAIL's failure record or a full-run FAIL's record), the re-run set is the record's failed checks plus its persisted `invalidated_checks` (written by `gate-invalidate` after a targeted P0/P1), the newly affected checks, the current keys the baseline never declared, and any explicit `--rerun` overrides; carried-over checks are the remaining `pass`/`carried` entries (see `framework/verification_scope.md` §Delta Runs → Failure recovery). A failure record whose per-check status map is absent or incomplete (legacy or malformed), or whose invalidated key cannot map to the current judgment surface, degrades the plan to the full coverage set — nothing is carried over. When the re-run covers every declared check, the plan covers the full scope — nothing is carried over. The incremental scope is reported by `gate-plan` before execution begins (the user must see what will be re-run and what will be carried over) and again in the final report.
- `Next step:` — the concrete command to run next with its reason; `None` when nothing further is needed. A finding's fix lifecycle has three states with fixed wording: `finding_open` → "Resolve the findings, then re-run the target-appropriate re-check command (`validate@{target}:check-{n}`; unit targets also `verify@{target}:{keyword}`) to confirm"; `fixed_pending_recheck` → "Fixes applied; re-run the target-appropriate re-check command to confirm." — only after the approved fix was actually written; `verified` → "Re-check passed." — only after a re-check confirmed the fix. A gate report is always produced before any fix is applied (nothing is implemented before the user approves the findings), so an actionable finding's report-time `Next step` is always the `finding_open` wording. Other guidance: both gates green → "if the design is finalized, run `promote@{target}`"; needs_decision → "awaiting your decision on {item}"; nothing further → `None`.

**Targeted runs:** end the report with the gate's targeted note ("This was a targeted check — no complete cache was written. Run `{gate}@{target}` for a complete ...") after the `Next step` line. If the result contains P0/P1, run `gate-invalidate` before reporting completion.

### Completion — Persist Gate Cache

A quality-gate run is complete only when its result is persisted and visible to `fresh`/`promote`. A report alone does not satisfy the gate.

- **Full (`validate@{target}` / `verify@{unit}`, `mode: full`, `basis: full`)** — complete only when the coverage run finished and its cache was written: (1) `specflowctl gate-plan --gate {validate|verify} (--unit {name} | --rule {id}) --target {candidate|stable} [--relationships NAMES|none] [--inputs-file PATH]` fixed the immutable input snapshot, local coverage set, and relationship scope (`--relationships` is required for unit runs; select names or `none`) **before any executor read input** (before mission assembly / sub-agent launch) — the input manifest adds evidence that every session may read but never creates a coverage key, and it must not be a repository-content file (keep it under the ignored `meta/plan_inputs/` or outside the repository); (2) the agent partitioned uncovered coverage into batches with the same kind and lens, reusing accepted public tasks and waiting for assigned tasks and, for each batch, generated a mission with `specflowctl gate-mission --run <run_id> --keys <k1,k2,...> --format prompt`, sent it verbatim to one independent reviewer, and recorded its report with `specflowctl gate-submit --run <run_id> --session <session_id> --keys <k1,k2,...> --report PATH` until `gate-status` reports full coverage; (3) when relationships are assigned or the primary pass produced findings, one independent final synthesis was generated with `specflowctl gate-mission --run <run_id> --final --format prompt` and recorded with `specflowctl gate-submit --run <run_id> --session cross --keys cross --report PATH`; a run with neither assigned relationships nor findings skips this step; (4) the coordinator ran `specflowctl gate-finalize --run <run_id>` and the cache `docs/specs/meta/validation/{unit|rule}/{name}/validate_result.md` | `verify_result.md` was written (the merged verify cache carries its `alignment` and `quality` sections); (5) `fresh@{target}` (`fresh --unit {name}` / `fresh --rule {id}`) shows the expected gate state (`FRESH` for `result: pass` / `blocking: false`, `BLOCKED` for `result: fail` / `blocking: true`). A candidate **full-run** FAIL writes a failure record (`result: fail` / `blocking: true`; validate/verify records declare `pass`/`fail` for every executed judgment) — the failure-recovery baseline. A finalize that finds any input changed since `gate-plan` is rejected and writes no cache. See `framework/validation_cache.md` §Write Rules and §Failure handling by gate role.
- **Delta (`revalidate@{target}` / `reverify@{unit}`, `mode: full`, `basis: delta` or `basis: repair`)** — the same coverage-run sequence with `specflowctl gate-plan --mode delta` (or `--mode repair` from a failure record): the plan derives stale/failed local checks and relationship checks mechanically, unions the coordinator-declared relationships, and records the carried-over judgments; `gate-finalize` merges the carried evidence from the baseline cache. See `framework/verification_scope.md` §Delta Runs.
- **Targeted (`:check-{n}` / `:{keyword}`, no complete cache)** — complete when the report is produced and, for P0/P1, the coordinator has run `specflowctl gate-invalidate` for every affected check key. Targeted runs intentionally do NOT create a gate run — `gate-plan` / `gate-mission` / `gate-submit` / `gate-finalize` are not run. A targeted PASS leaves cache state unchanged. A targeted P0/P1 deletes a pass cache or persists `invalidated_checks` on a failure record and invalidates any matching open run. For verify, the same locked transition invalidates the contradicted immutable evidence, so consuming caches and open runs must recheck the affected judgments. It never publishes a complete result cache or satisfies the promote gate. The report ends with `This was a targeted check — no complete cache was written. Run {gate}@{target} for a complete ...`.

**Self-check:** `ls docs/specs/meta/validation/{unit|rule}/{name}/` and `fresh@{target}` (unit: `fresh --unit {name}`, rule: `fresh --rule {id}`).
==ATOM_END:report_skeleton==

### Body format (verify)

`alignment` lens body:

```
Items:
  - {item.id}: ALIGNED | MISMATCH (type) [P0|P1|P2|P3] | CANNOT_DETERMINE — code references
Scope: PASS | FAIL — findings
Integrity: PASS | FAIL — findings
Coverage:
  - items_with_deterministic_evidence: N/M
  - items_reading_only: N
First-principles divergence analysis: (only if any MISMATCH)
  - {item.id}: {suggested direction} [P0|P1|P2|P3] — {rationale}
    User verdict: ...
    Next step: ...
```

`quality` lens bodies follow `framework/shared_judgments.md`: public facts and unit design are file tasks; architecture is one unit task. Architecture body:

```
Unit: architecture:{unit}
Architecture assessment:
  conclusion: acceptable | needs_attention | unacceptable — {basis}
  module_boundaries: {assessment} — {basis}
  responsibility_organization: {assessment} — {basis}
  dependency_clarity: {assessment} — {basis}
  abstraction_level: {assessment} — {basis}
  extension_landing_points: {assessment} — {basis}
  engineering_patterns: {assessment} — {basis}
gate_findings: none | [P0|P1] {finding};
Suppressed by spec (N):
  - {suppressed finding} — suppressed by {spec context}
```

The `[P0|P1|P2|P3]` on MISMATCH lines is the severity assigned by the session that raises the finding and carried in the MISMATCH block's own `Severity:` line. Deterministic Step 1 table rows keep their contract-decided grade; the final synthesis may only raise a retained finding's canonical severity conservatively — never lower it (see `framework/severity_policy.md` §9).

The `Target:` line from earlier formats is now the `{layer}` field in the unified skeleton header (`candidate` | `stable`).

---

## Step 1 — Structural alignment

**Purpose:** Cross-reference every static structure declared in the spec body and appendices against the actual implementation code. This catches missing interfaces, wrong signatures, and inconsistent data definitions that acceptance items alone might miss.

**Why this exists:** Acceptance items describe "what to verify" but the spec (body + appendices) describes "how it's built." If a protocol definition or data structure in the spec differs from what's implemented, verify must catch it — even if the acceptance items pass.

**Execution steps:**

1. Read the full spec body AND all non-exempt appendix files (main flow, protocols, data contracts, error handling, state machines, API definitions from appendices)
2. Extract all statically verifiable structural declarations from both main spec and appendix content:

| Declaration type | What to extract | How to verify in code | Semantic role | Match semantics | Severity if missing |
|---|---|---|---|---|---|
| API endpoints / handlers | path, method, request/response types | Search for route registration, check handler function signature | Contractual | Exact match | P0 |
| Function signatures | name, parameters, return types | Search for function definition, verify signature | Contractual | Exact match | P0 |
| Data structures | struct/type definitions, field names, types, tags | Search for type definition, check spec-declared fields exist in code | Descriptive | Subset match (spec ⊆ code) | P1 |
| Enums / constants | allowed values, string representations | Search for const/enum block, verify values exist | Contractual | Exact match | P0 |
| Error type names | error type identifiers, sentinel variables | Search for error definition, check usage in error paths | Contractual | Exact match | P0 |
| Error messages | user-facing message strings | Search for message constants/format strings | Descriptive | Subset match (spec ⊆ code) | P2 |
| State machines | state values, transition triggers | Search for state type, switch/case blocks | Contractual | Exact match | P0 |
| Configuration keys | key names, default values | Search for config struct or usage | Descriptive | Subset match (spec ⊆ code) | P2 |

The **Semantic role** column categorizes each declaration by its purpose in the spec:

- **Contractual**: The declaration describes behavioral constraints the system must obey. Code must match exactly; any deviation affects functional correctness.
- **Descriptive**: The declaration describes the minimum contract of the implementation. Code may provide more information without violating the contract.
- **Annotative**: The declaration is auxiliary documentation (e.g., scope, rationale); inaccuracy does not affect functional correctness.

The **Match semantics** column defines the comparison protocol between spec and code:

- **Exact match**: Everything the spec declares must exist in code in exactly identical form. Any deviation (missing field, different signature) → MISMATCH, using the severity from the row.
- **Subset match (spec ⊆ code)**: The spec describes the minimum contract for the declaration. Code must contain all spec-declared fields/values; extra code fields/values do not constitute a MISMATCH — record as a note. Match rule: spec ⊆ code.

3. For each declaration found in the spec, locate the corresponding implementation. Use the **Match semantics** from the extraction table above to determine the correct outcome:

```
IF declaration exists in spec AND matching implementation found:
  - Read the declaration's row in the extraction table:

    Exact match → Verify all spec-declared fields/signatures/values exist in code
                  AND are identical.
                  All match → ALIGNED.
                  Any mismatch → MISMATCH (severity from table row).

    Subset match → Verify all spec-declared fields/values exist in code.
                   All exist → ALIGNED.
                     (extra code fields do not constitute a MISMATCH —
                      record as a note)
                   Spec-declared fields missing from code → MISMATCH
                     (severity from table row).

IF declaration exists in spec BUT no matching implementation found:
  → MISMATCH (severity from table row)
    — spec describes something code doesn't have.

IF a complete declaration exists in code BUT has no correspondence
   in any spec declaration:
  → Do NOT report here. Defer to Step 5 (surplus detection).
  → Exception: Subset match declarations whose spec-declared fields are all
    present in code are ALIGNED (extra code fields are not surplus).
```

4. **Reverse direction:** code→spec surplus discovery is owned by Step 5 Part A (the design-surface scan with its implementation-detail filter). Do not run a second structure-level reverse scan here — Step 5 Part A reads the complete associated files, attributes structures to responsibilities (the attribution rules there cover shared files and preserve scoping), and defers candidates to Step 7. The subset-match exception from step 3 stands on its own: subset-match declarations whose spec-declared fields are all present in code are ALIGNED (extra code fields are not surplus).

**PASS (ALIGNED):** All spec body declarations have structurally consistent implementations

**FAIL (MISMATCH):** Structural differences found between spec and code — defer classification to Step 7

**Check method:** Spec body × implementation code — structural cross-reference in the spec→code direction (the code→spec direction is Step 5 Part A)

---

## Step 2 — Acceptance alignment

**Purpose:** For each acceptance item, verify that the code structurally satisfies the pass_condition using static analysis. This is not about running tests — it is about confirming the code contains the structures, functions, and patterns needed to satisfy the condition. For appendix content, cross-reference technical claims (API contracts, data type definitions, configuration keys) against code — appendix contract content always has a corresponding acceptance item (validate Check 5a enforces this, see `framework/spec_writing_guide.md` §4), so the item set is the complete contract surface.

**Why this is achievable:** A pass_condition like "Returns HTTP 201 with id and created_at" can be verified by checking the handler code returns 201 status and includes those fields in the response struct. The subagent cannot verify the code *works correctly*, but it can verify the code *is structured correctly*.

**Execution steps:**

For each acceptance item in the target spec (appendix content is read as the item's context, not as an independent scan object — validate Check 5a enforces that every appendix contract has a carrying acceptance item, so the item set is the complete contract surface and appendix claims are verified through the items that carry them):

1. Read `implementation_surface`, `verification_surface`, and `affects.files` to locate the implementation
2. Parse the `pass_condition` and extract specific verifiable assertions:

| pass_condition type | What to check in code | Verifiable? |
|---|---|---|
| "Returns HTTP {status}" | Handler returns that status code | Yes — search for status code in handler |
| "Returns {field} in response" | Response struct has that field | Yes — check response type definition |
| "Calls {function} when {condition}" | Function call exists in correct branch | Yes — read conditional block |
| "Returns error when {condition}" | Error return in conditional path | Yes — read error handling path |
| "Rate limit is {n} req/min" | Rate limiter configured with that value | Yes — read rate limiter config |
| "{algorithm} produces correct result" | Function exists with algorithm | Partial — structure exists, correctness not verifiable |
| "System handles {n} concurrent users" | N/A without load testing | No — CANNOT_DETERMINE |

3. For each verifiable assertion, locate the corresponding code and compare:
   - Does the handler exist at the expected path?
   - Does it have the right status code?
   - Does the response type include the expected fields?
   - Are the error types defined and used in error paths?

4. For each verifiable assertion, report the grep command or file
   check used as evidence. Every ALIGNED claim must include a specific
   deterministic check:

   ALIGNED (with deterministic evidence):
     grep -n "return 201" user.go
     → line 42: w.WriteHeader(http.StatusCreated)
     → handler returns 201 (confirmed by grep command)

   Insufficient — no grep command cited, reading only:
     MUST be reported as CANNOT_DETERMINE

5. If an assertion cannot be verified with a deterministic command
   (e.g., "error message is user-friendly"), report as CANNOT_DETERMINE.

   {item.id}: CANNOT_DETERMINE
     - pass_condition: "error message is user-friendly"
     - Reason: requires human judgment, no deterministic grep available

Per-item report format:

```
{item.id}: ALIGNED
  - evidence: grep -n "201" src/api/user.go → line 42
  - deterministic: true

{item.id}: MISMATCH
  - Spec pass_condition: "returns 201"
  - Implementation: handler at src/api/user.go:42 returns 200

{item.id}: CANNOT_DETERMINE
  - pass_condition: "handles 1000 concurrent requests"
  - Reason: requires load testing, not statically verifiable
```

**PASS (ALIGNED):** All items have structurally consistent implementations

**FAIL (MISMATCH):** One or more items have structural inconsistencies — defer classification to Step 7

**CANNOT_DETERMINE:** Items that require runtime verification — note them for human review

**Check method:** Acceptance item pass_condition × implementation code — assertion-level structural cross-reference

**Test design sub-check (for verification_type: testable items only):**

Ownership note: the `alignment` lens records whether tests exist for an item (Part A coverage as annotation evidence); the **quality** of those tests (Part B meaningfulness — mock density, assertion authenticity incl. tautology and mock-through signals, happy-path-only) is owned by the `quality` lens (see §Lenses and boundary ownership and the quality-lens standard above). A test-quality concern is never an `alignment` finding.

This sub-check has two parts: **A — coverage completeness** (tests exist for implied scenarios) and **B — test meaningfulness** (existing tests are genuine). The agent runs both parts and reports findings per acceptance item.

**Language-agnostic approach:** The agent reads test files and self-identifies the testing framework (mock libraries, assertion libraries, test runner conventions) rather than relying on a hardcoded language list. When a test framework or assertion style is unfamiliar, the agent reports CANNOT_DETERMINE rather than guessing.

For a coverage run, the coordinator locates relevant test and code-context file paths before `gate-plan`, without assessing their behavior, and lists every discovered file in the input manifest (`--inputs-file`). Include files already in the declared implementation surface so delta/repair can detect evidence missing from the baseline. This discovery covers every acceptance item in full, delta, and repair modes. The independent alignment reviewer reads and declares the relevant tests from its session `read_refs`; a required test or context file missing from those refs is an incomplete session, not evidence that no test exists.

---

### Part A — Coverage completeness

Identify significantly implied but missing test scenarios:

1. **Happy path:** Does a test exist that exercises the primary success scenario? If the pass_condition describes a success outcome and no test covers it → CONCERN (possible untested core behavior)
2. **Input variants, business rules, dependency failure:** Does the description or pass_condition strongly imply a scenario (e.g. "register" implies "email already exists", "create order" implies "invalid product ID") that has no corresponding test? If a reasonable developer would expect a test for that scenario and none exists → CONCERN

Reference: `framework/test_decomposition_standard.md` provides decomposition methodology for deeper scenario discovery but is not required reading for this sub-check.

Part A is **not** an exhaustive coverage audit. It flags obvious omissions. A single acceptance item may produce zero, one, or several test scenarios depending on its content. If the implementation is in a language or framework where tests are not written in the expected location, report CANNOT_DETERMINE.

---

### Part B — Test meaningfulness

Existing tests are not automatically meaningful. This part checks whether the tests that *do* exist are structured as genuine validation — or are ritual tests that pass regardless of implementation correctness.

**Execution steps:**

For each acceptance item with `verification_type: testable`, locate the corresponding test files. For each test function found, apply the following checks:

#### B1 — Mock density

**Method:** Identify all import paths in the test file. Separate mock/stub/fake imports (e.g. `testify/mock`, `gomock`, `jest.mock`, `sinon`, `unittest.mock`, `pytest-mock`) from regular imports. Calculate the ratio.

```
mock_ratio = mock_imports / total_imports
```

**Language-agnostic detection:** Agent reads imports and identifies mock/stub/fake libraries by name convention and usage context rather than a hardcoded list. Libraries whose primary purpose is creating test doubles are counted as mocks.

| Ratio | Report |
|-------|--------|
| < 80% | No concern |
| ≥ 80% | CONCERN — "Mock density is {n}%. Most dependencies are mocked; only orchestration is exercised." |
| 100% | CONCERN — "Every dependency is mocked. Test exercises wiring only, not real behavior." |

Mock density above 80% is a **signal**, not a verdict. The agent uses it in combination with other Part B checks. A well-written test with 100% mocks that verifies interaction patterns (e.g. "did the service call repository with the correct transformed data") is meaningful. A test with 100% mocks that calls a function and asserts a tautology is not.

#### B2 — Assertion authenticity

**Method:** For each test function, agent reads the full body and evaluates whether the assertions genuinely verify the outcome implied by the test name and acceptance item — i.e., whether the test would fail if the implementation logic it names were broken.

Checklist per test function:
- Does the test have at least one assertion?
- Does the assertion target an actual output value (return value, state change, side effect) rather than a fixed/tautological expression?
- If the test name describes an error scenario ("returns error when email exists"), does at least one assertion check the error (type, message, presence)?

**Named signals** — two recurring ways an assertion fails this check. Both are evaluated with the same language-agnostic reasoning: the agent reads the assertion call and its arguments and asks whether it could fail under any reasonable code change.

- **Tautological assertion** — the assertion always passes regardless of implementation state:
  - Is the asserted value an unconditional literal? (`assert.Equal(42, 42)` → always passes)
  - Is the asserted value the test infrastructure itself? (`assert.NotNil(t)` where one `t` is `*testing.T` → never nil)
  - Is the assertion checking a property guaranteed by the test setup rather than the implementation? (mock returns a fixed value, then the assertion checks that same fixed value without transformation)
  - A single tautological assertion in a healthy test file may be accidental; multiple across an acceptance item's tests → stronger ritual-testing signal.
- **Mock-through** — the mock's return value passes through the function without transformation, validation, or conditional logic, and the assertion checks that same (or derived) value: the test exercises only the mock, not the implementation. Trace the data flow across mock setup → function call → assertion, then apply the decision test:
  - "Would this test fail if the function body were replaced with a no-op / passthrough?" → No, it passes → CONCERN (only the mock is exercised).
  - "Would this test fail if the validation/transformation logic were broken?" → Yes → No concern (implementation logic is exercised).

**Reports:**

```
{item.id}: CONCERN — Test "TestRegister_DuplicateEmail" describes a conflict scenario
  but contains no error assertion. The test calls the register function but only
  asserts NoError. The conflict logic is never verified.

{item.id}: CONCERN — Test "TestRegister_Success" exercises mock passthrough only.
  Mock returns User{ID: 1, Name: "a"}, service returns it unchanged,
  assertion checks ID == 1. The test passes even if all business logic
  is removed.
```

**Signal usage:** A test missing a meaningful assertion for its stated purpose is a strong indicator of ritual testing. Even one such test per acceptance item warrants a CONCERN.

#### B3 — All-happy-path detection

**Method:** After Part A has confirmed a happy path exists, count all test functions associated with the acceptance item. If every test exercises a success scenario and none exercises error/invalid/edge paths → CONCERN.

This is distinct from Part A's check: Part A checks whether a *specific implied scenario* is missing. B3 checks the *overall profile* of existing tests — a complete lack of negative testing.

```
{item.id}: CONCERN — All {N} tests are happy-path only. No test exercises
  validation rejection, business rule conflict, or dependency failure.
```

---

### Part A/B integration

Part A and Part B findings are reported together per acceptance item:

```
{item.id}: ALIGNED
  - evidence: grep -n "201" src/api/user.go → line 42
  - deterministic: true
  Part A: No concerns
  Part B: One concern
    - B1 Mock density: 60% — no concern
    - B3 All happy path: CONCERN — 3 tests, all success scenarios, no error path
```

Part B findings are recorded under the item in the verify output as CONCERN-level annotations. They do not change the item-level verdict (ALIGNED / MISMATCH / CANNOT_DETERMINE).

**Edge case:** If test files are not in a language the agent can parse with confidence, or if the test framework is unfamiliar, report CANNOT_DETERMINE for all Part B checks on that acceptance item. Do not guess.

**Edge case:** If acceptance item has no tests at all, Part A flags the missing scenarios; Part B is skipped entirely (no tests to evaluate).

---

## Step 3 — Scope accuracy

**Purpose:** Cross-reference each acceptance item's declared implementation files — `implementation_surface` (a file, or a directory expanded to its repository-content files) and `affects.files` — against the actual implementation. This catches undeclared scope and missing declarations.

**Execution steps:**

1. For each acceptance item's declared implementation files — its `implementation_surface` value (a file, or a directory expanded to its repository-content files) plus every `affects.files` entry:
```
- Read each declared file
- Does the file contain implementation relevant to the pass_condition?
- Is the nature of the change consistent with the behavior described?
  (If item says "add login handler" but the file only has imports → flag)
- Does this file participate in the declared behavior? Shared implementation is permitted;
  check this unit's requirements and preserve related stable requirements independently
  (see `framework/shared_judgments.md`).
```

2. For each acceptance item with `affects.rules`:
```
- Search for each declared rule name or pattern in the implementation files
- Is the rule being used? How?
- If the rule is not found in the implementation → flag (possible undeclared scope)
```

3. For each acceptance item with `affects.dependencies`:
```
- Search for each declared dependency in the implementation
- Is the dependency used? If not → flag
```

4. Cross-reference: compare the declared implementation files (`implementation_surface` expansion plus `affects.files`) against the set of files that contain actual implementation related to this acceptance item:
```
- Files with relevant code but not declared → flag (under-declared scope)
- Declared files with no relevant code → flag (over-declared scope)
```

**Scope boundary for non-implementation files:**
- Test files are not scope violations: the declared implementation files (`implementation_surface`/`affects.files`) declare implementation scope; a relevant test file absent from them is recorded in `Dependency scope`, not reported as under-declared scope.
- Dependency files (read for context but not part of the implementation) are likewise recorded in `Dependency scope`, never as scope findings.

**PASS:** All declared implementation files are accurate and complete

**FAIL (scope MISMATCH):** Undeclared scope or inaccurate declarations found — defer classification to Step 7

**Check method:** declared implementation files (`implementation_surface` × `affects.files`) × actual implementation — triple cross-reference (files, rules, dependencies)

---

## Step 4 — Retirement verification

**Purpose:** For replacement candidates, verify old code is fully removed with no remaining references.

**Execution steps:**

1. Check if any acceptance item has `verification_type == inspectable` AND `evidence_requirements` includes `old_code_deleted` and `no_remaining_refs`. If not, skip this step.
2. If `replacement`, for each acceptance item with `verification_type: inspectable` and `evidence_requirements` containing `old_code_deleted` and `no_remaining_refs`:
```
- Identify the old code paths from the spec (or from the replacement nature of the change)
- Verify old code files/directories no longer exist
- Search for remaining references to:
    - Old function names
    - Old import paths
    - Old configuration keys
    - Old module/package names
```

**PASS:** Old code removed, no remaining references

**FAIL:** Old code or references still exist

**Check method:** Replacement scope × file existence × grep for remaining references

---

## Step 5 — Implementation integrity & surplus detection

**Purpose:** Discover code designs within this unit's responsibility that are not declared in its spec (code surplus) and assess non-runnable items. Do not classify surplus findings yet — defer to Step 7. The reverse-check responsibility boundary from Step 1 also applies here. A preserve task assesses only the protected stable requirement and the designs implementing or constraining it; unrelated behavior in the same file does not become a protected mismatch.

**Execution steps:**

### Part A — Code surface surplus analysis

This analysis operates at the **design surface** level — package structure, core types, entry points, cross-cutting mechanisms — not individual function signatures. The goal is to systematically discover code designs that exist but are not recorded in the spec.

#### 1. Discover the code design surface

Execute each step below in order and record findings:

```
a. Read the directory structure
   - List all packages/modules under the implementation_surface paths
   - Trace responsibilities through implementation, callers, and data flow;
     names and file organization alone do not prove responsibility ownership

b. Read each package's public interface
   - What types/interfaces/structs does the package export?
   - What are the main entry points (constructors, factory functions, handler registrations)?
   - What are the core abstractions?

c. Read initialization/boot code
   - How are components wired together (main, init, bootstrap)?
   - What gets registered at startup (routes, middleware, subscribers, scheduled tasks)?
   - What cross-cutting mechanisms are initialized (cache pools, connection pools, rate limiters)?

d. Trace cross-file data flow
   - How do components communicate (direct call, events, message queue)?
   - Are there event buses, middleware chains, pipelines not mentioned in the spec?
   - Are there interception points (hooks, decorators, middleware) not described in the spec?

e. Identify cross-cutting mechanisms
   - Caching layer
   - Retry logic
   - Rate limiting
   - Logging / observability
   - Circuit breaker
   - Background / scheduled tasks
   - Feature flags
```

#### 2. Match against the spec surface (main spec + appendices)

```
For each design construct discovered in step 1:
- Establish whether it implements or constrains this unit's responsibility
  (for preserve, the assigned protected stable requirement).
- Record evidence and exclude independent responsibilities outside that scope.
  Another unit sharing the file is not, by itself, proof of exclusion.
- If evidence is missing, replan with the needed input before judging surplus.

For each remaining in-scope construct:
- Does the spec body (protocol, architecture, responsibility sections) describe it?
- Does any appendix file describe it (API contracts, data types, component trees)?
- Does any acceptance item's description or pass_condition imply it?
- Is it referenced in the spec's terminology or data contracts?

If a construct has no correspondence in either the main spec or any appendix → record as surplus candidate.
```

#### 3. Filter: is it a real design decision or an implementation detail?

```
For each surplus candidate, determine:

Is a real design decision (must report):
- Has its own type/abstraction → yes
- Other code depends on it → yes (has consumers, not dead code)
- Has tests → strong signal it is intentional design
- Can be independently described as a responsibility/mechanism → yes
- Part of the public API surface (exported, cross-package boundary)

Is an implementation detail (ignore):
- Merely a utility/helper function
- No independent architectural significance
- Does not change the system's responsibility boundaries
- Private/internal type with no external consumers
- **Behavior is already covered by a Contractual declaration in the spec**
  (e.g., EventSender interface is an internal realization of the
  StateMachine already described in spec §3.3 — not surplus)
```

#### 4. Report

```
Zero surplus → "Unit code design surface fully covered by spec — no surplus within its responsibility"

Surplus found → report each as MISMATCH (type: surplus) with:
- Design description (what it is)
- Code location (file and line)
- Evidence (why judged as a real design decision)

Do not classify the resolution direction — defer to Step 7.
```

2. **Non-runnable assessment:**
```
- Count acceptance items with runnable: no
- If all items are runnable: no → report: "All acceptance items marked runnable: no — verify cannot confirm alignment"
- If some items are runnable: no:
    - List them
    - For each item, verify not_runnable_reason is present
    - If missing → flag as concern: "Item {id} has runnable: no but no not_runnable_reason"
    - If reason is present, does it reference external evidence?
      (e.g. issue/PR link, dependent system documentation, pending integration entry point)
    - If not → flag as concern: "Item {id} reason is self-attested — no external evidence"
    - Does the remaining runnable set provide meaningful coverage?
    - If not → flag as concern
```

**PASS:** No surplus design found; non-runnable items have documented reasons

**FAIL (MISMATCH):** Surplus design found — defer classification to Step 7

**Quality concern:** All or most items are non-runnable; runnable coverage is insufficient

**Quality concern (reasoning):** One or more non-runnable items are missing not_runnable_reason or lack external evidence

**Check method:** Implementation code design surface × spec body — reverse cross-reference (code-to-spec design direction)

---

## Step 6 — Stub & Placeholder Scan

**Purpose:** Systematically scan for stub and placeholder patterns — in the spec's implementation mapping and in implementation files. This is a deterministic check — running it twice produces identical results. It catches incomplete implementations that pass structural checks (the structure exists) but are placeholders. Debt markers (TODO/FIXME/XXX/HACK comments) are NOT scanned here — they are owned by the `quality` lens (Dimension 7, see §Lenses and boundary ownership).

**Execution steps:**

1. **Spec-side placeholder check (before collecting files):** For each acceptance item, read the `implementation_surface` value:
   - Value is `<pending>` → MISMATCH: the design-first placeholder was never backfilled — the mapping must be completed before verify can locate the implementation (do not classify yet — defer to Step 7)
   - Value is not `<pending>` but resolves to no file — a missing path, or a directory with no repository-content files (the files Git tracks plus untracked files that are not ignored) → MISMATCH: the declared mapping points at no implementation (do not classify yet — defer to Step 7). `gate-plan` already rejects such a spec, so this state means the surface changed after planning — the finalize snapshot comparison reports the divergence as well

2. Collect all implementation files from `implementation_surface` paths and `affects.files` across all acceptance items. If `affects.files` is incomplete, also collect files from the spec body's implementation references.

3. For each file, run these commands and record findings:

   a. Stub/empty patterns:
   ```bash
   grep -n "return null\|return \[\]\|\bplaceholder\b\|not.*implement" <file>
   ```

   b. Empty handler bodies:
   ```bash
   grep -n "return Response.json({})\|w.WriteHeader(204)" <file>
   ```

4. Per-file result:
   ```
   {file}: CLEAN | STUB_FOUND
     - line 12: return Response.json({}) (empty_response)
   ```

5. Classify each grep hit: **RELEVANT** (a real stub or placeholder in implementation code) or **IRRELEVANT** (idiomatic constructs — e.g. Go `return nil`, error sentinels, test fixtures). Only RELEVANT hits are MISMATCH.

6. Any RELEVANT stub finding is a MISMATCH — code has placeholder where real implementation is expected (do not classify yet — defer to Step 7)

**PASS:** No stubs or placeholders found

**FAIL (MISMATCH):** One or more files contain stub patterns — defer classification to Step 7

**Check method:** grep — deterministic, outputs are identical across runs

---

## Step 7 — First-principles divergence analysis

**Purpose:** When Steps 1-6 detect mismatches, determine the root cause and correct direction using first-principles reasoning — not presence-based heuristics. The independent item reviewer completes this analysis before submitting its alignment report.

### How it works

For each MISMATCH detected in Steps 1-6, the assigned **read-only item reviewer** completes Step 7 in the same session. Analyze each item independently, including when several items share a batch. The initial report includes the verdict, root cause, suggested direction, severity, confidence, evidence and affected keys; acceptance completes both alignment and divergence analysis.

> **Full mode note:** In full mode the coverage set contains `item:<unit>:<item>`, `preserve:<unit>:<item>`, `code:<file>`, `design:<unit>:<file>` and `architecture:<unit>`. The agent batches uncovered tasks into sessions with the same kind and lens (see `framework/verification_scope.md` §Coverage model). A session that covers item(s) reports the alignment verdicts; each `MISMATCH` also names its affected keys and includes the completed Step 7 analysis.

> **Delta mode note:** A delta/repair run fixes the re-run coverage set (the stale keys plus the current keys the baseline never declared). Carried items bring their prior structured judgment into the run. The final synthesis consumes the new resolved judgments and the carried judgments together.

### Item reviewer protocol

Before reviewing uncovered `item` or `preserve` keys in the `alignment` lens, the main agent generates `specflowctl gate-mission --run {run_id} --keys {keys} --format prompt` and sends its output verbatim to the assigned independent read-only reviewer. The mission supplies Steps 1-7, exact session input paths, the report contract and submission command. The reviewer performs alignment and any required divergence analysis before one `gate-submit`. Accepted keys are terminal: do not generate a second analysis mission for them. The final synthesis consumes their accepted reports; the coordinator does not assemble another analysis prompt or copy a mismatch table.

The item reviewer locates and analyzes the mismatch while executing Steps 1-7. It reads the mismatch point's enclosing function or structure, direct callers and callees, relevant tests, the item's spec section, sibling items, and shared definitions (error codes, types, enums, data models, rationale) within its session read refs. When unsure whether a region informed the judgment, it declares more in `Dependency scope:`. If a required file is not in `read_refs`, it returns `Verification could not complete — missing read ref: <repo-relative path>` without a verdict. The coordinator does not submit that report; it discovers the missing path, re-plans with all accumulated manifest entries, and re-executes the replacement run. The reviewer includes the root cause and repair direction in the initial item report. It must not modify files, run state-changing commands, or launch sub-agents.

For surplus mismatches, the first-principles analysis additionally evaluates:
- Is this a genuine design decision that belongs in the spec? → **spec_gap**
- Does this conflict with the spec's declared architecture or boundaries? → **boundary_violation**
- Does this duplicate or replace something the spec already describes differently? → **coding_gap**

**Context scope — the sub-agent MUST read:**

| Context | What to read |
|---------|-------------|
| Spec context | The section/group containing the item, sibling items, shared definitions (error codes, types, enums, data models), rationale in section headers, and relevant appendix sections |
| Code context | The implementation function body where the mismatch occurs, surrounding logic, related helpers, and callers |
| Tests | If test files exist for the relevant code module, read their assertions for the mismatched behavior |
| Appendix context | If the mismatch involves an appendix claim, read the full appendix file to understand the claim's context and relationship to the main spec |

**Analysis instruction (first-principles framework):**

The sub-agent follows this reasoning chain. Each step must be answered explicitly before moving to the next:

```
1. What is the system supposed to do at this point?
   - Read spec context (section, sibling items, shared definitions, rationale)
   - Extract the design intent — what problem is this declaration trying to solve?

2. What does the code actually do?
   - Read code context (function body, surrounding logic, callers, related helpers)
   - Extract the code's intent — why does it behave this way?

3. Compare intents:
   - Do spec and code agree on the goal but differ on implementation?
   - Do they disagree on the goal itself?
   - Is one side clearly wrong (typo, dead code, outdated reference)?
   - Does one side handle edge cases the other misses?
   - Truth ownership / shadow spec check: Does the mismatch involve fields, parameters, or internal structures of a collaborating unit? If so, does the collaborating unit export them in its formal behavior carriers (acceptance items or protocol appendices)? If the spec is enumerating private/volatile internals of another unit without an exported contract anchor, the spec is a shadow specification (see `framework/spec_writing_guide.md` §14). File association check: does the declared implementation participate in this item's behavior? Shared files are permitted; retain the unique source for behavior and shared agreements.

4. Root cause analysis (choose the best fit):
   - Code is incomplete — spec intent is clear, code hasn't caught up
   - Spec is stale — code has evolved, spec wasn't updated
   - Protected record stale (preserve tasks) — the protected stable requirement's declared implementation mapping no longer resolves while its behavior is still implemented; the protected unit's record lags and only its own promote refreshes it
   - Shadow specification / over-specification — the spec mirrored private, internal, or obsolete implementation details of another unit that have evolved or been removed
   - Invalid file association — a declared file does not participate in this item's behavior
   - Design divergence — both sides made different valid trade-offs
   - Accident — bug, typo, copy-paste error
   - External dependency — blocked on something outside this unit

5. Recommend direction:
   - spec_gap: code's behavior is correct, spec needs updating
   - code_gap: spec's intent is correct, code needs updating
   - needs_design: neither side is clearly right — the design itself needs rethinking
   - blocked: the mismatch depends on an external input or unresolved decision
   - **Shadow specification anti-regression rule (MANDATORY):** If the root cause is a shadow specification (non-owner unit hardcoding unexported/private parameters of a collaborating unit), the recommended direction MUST be **spec_gap** (update the peripheral spec to restore behavioral abstraction or use public contract anchors). **NEVER recommend code_gap to re-introduce removed parameters or dead code into a collaborating unit solely to satisfy a shadow spec.**
   - **Protected record drift (MANDATORY, preserve tasks):** When the protected stable requirement's declared implementation mapping no longer resolves while its behavior is still implemented, the item verdict is MISMATCH — never ALIGNED. Root cause is `stale`, the suggested direction is `spec_gap` (the protected unit refreshes its own record), and the fix names that stale mapping. The final synthesis routes the finding to the protected unit as peer-owned record drift (`framework/verification_scope.md` §Deferred findings); it does not block the current unit. A protected requirement whose behavior is not implemented anywhere is not record drift — it stays a gate-driving mismatch of the current unit.

6. Confidence:
   - high: clear evidence supports one direction
   - medium: evidence leans one way but not definitive
   - low: conflicting signals, cannot determine with confidence
```

**Signal layer (metadata evidence):**

The sub-agent MAY also collect the following metadata to support its reasoning. Each evidence item has a weight — the sub-agent sums weighted evidence to determine confidence if needed:

| Evidence | Favors fixing | Weight |
|----------|--------------|--------|
| Spec is **stable** (approved) | Code (spec is contract) | Strong |
| Spec is **candidate** (WIP) | Spec (code is evolving) | Strong |
| Validate cache is fresh | Code (spec was recently checked) | Strong |
| Code has tests for the behavior | Spec (behavior is intentional) | Strong |
| Spec modified after code | Code (spec reflects latest thinking) | Moderate |
| Code modified after spec | Spec (code evolved past spec) | Moderate |
| Code behavior referenced by other modules | Spec (other code depends on it) | Weak |

If version control is available, query `git log` for file timestamps. If no test files exist, skip test evidence. Do not infer "no tests" as evidence for anything.

The signal layer is supplementary — it does not override first-principles reasoning. Its primary use is to provide confidence context when the content-level analysis alone cannot reach high confidence.

**Output format:**

```
Item: {id}
────────────────────────────────────
Spec intent: {what the spec is trying to achieve, with context}
Code intent: {what the code does and why}
Problem: {one-sentence mismatch statement naming the item and the concrete declaration or behavior — no anchors}
Evidence:
  - spec: {verbatim spec text} — {source: item id, section, or appendix}
  - code: {verbatim code} — {file:line}
Impact: {what goes wrong or stays undecidable if this mismatch is not resolved}
Fix: {the concrete repair action, naming the files it touches}
# or
Decision: {the question the user must answer}
Options:
  - {candidate option}
Root cause: {incomplete | stale | shadow_spec | divergence | accident | blocked}
Suggested direction: {spec_gap | code_gap | needs_design | blocked}
Files involved: {spec and/or code file paths the suggested fix would touch}
  - List every file the fix would modify; the main agent uses this for batch classification scope evaluation
Dependency scope: {the files this alignment judgment actually depended on}
  {item.id}: {file}: {declaration}   # declaration = `acceptance_item:{item.id}` for the item's own spec block, a section-region heading, 1-based closed line ranges, or "all"
  - These declarations travel verbatim into the verify cache: the item's own spec block is declared as `acceptance_item:{item.id}` (the item region — located by id, so editing or reordering other items never stales this judgment), section-region headings name regions of the unit's own main spec (recorded per item in the cache's `checks` mapping), line ranges and `all` declare code-file scope; `gate-submit` validates the declarations against that session's `read_refs`, and `gate-finalize` computes the CIDs and writes the cache. The declared regions must cover every region this judgment depended on, including called functions and referenced structures
Severity: {P0 | P1 | P2 | P3}
  - Derive from the declaration's table row in Step 1 (severity if missing).
  - For surplus findings: determine by the construct's semantic role —
    public API = P0/P1, internal implementation detail = P2.
  - For scope findings (Step 3): annotative role → default P2.
  - For acceptance findings (Step 2): map the pass_condition's semantic role
    to the Step 1 severity table — contractual assertions (status codes,
    response fields, error codes) take the table row's severity for the
    declaration type they express; descriptive assertions default to P2.
  - For stub findings (Step 6): a RELEVANT stub or placeholder in production
    code defaults to P1; a stub in test-only or tooling code defaults to P2.
Confidence: {high | medium | low}
Rationale: {2-3 sentence first-principles reasoning chain}
Signal layer: {condensed metadata summary, if collected}
```

The `Problem:` / `Evidence:` / `Impact:` / `Fix:` (or `Decision:`) fields are the finding block: the tool renders them as the finding's detail lines in the unified report skeleton (§Output Format), followed by the `root_cause:` / `direction:` / `confidence:` extra lines. `Problem:` answers the comparison step; `Evidence:` quotes each present side verbatim, and when one side is absent — a surplus construct has no spec declaration, a missing implementation has no code — its sub-line states the absence and the searched scope; exactly one of `Fix:` and `Decision:` is required and must match the suggested direction (spec_gap / code_gap → `Fix:`; needs_design / blocked → `Decision:` with at least one `Options:` entry).

**Failure path:** an item reviewer that cannot complete Steps 1-7 reports:
"Verification could not complete — {reason}" without a verdict. Do not submit
an incomplete report; resolve the missing input and re-plan when necessary.

**Constraints:**

- Read-only: MUST NOT modify files
- Independent: each mismatch is analyzed separately, without reference to other mismatches
- If version control history is available, MAY query `git log` for the relevant files. If not available, skip timestamp evidence — confidence capped at medium.

### Batch classification (main agent)

When MISMATCH items exist, the main agent classifies each finding into a **batch group** or a **decision group** before presenting. Classification aggregates fields the item reviewers already returned with their Step 7 analyses (root cause, suggested direction, confidence, severity, files involved) — it adds no new analysis; for batch group candidates it only runs lightweight assertion re-verification (see Assertion re-verification below).

**Activation threshold:** Batch grouping is enabled only when the total MISMATCH count ≥ 10 AND the batch group would contain ≥ 3 items. Below the threshold, present findings flat (§Summary format without grouping).

**Batch group eligibility (ALL must hold):**

A. **Direction is unambiguous:**
   - Root cause is `accident` or `incomplete` (exclude `divergence`, `blocked`, `stale`, `shadow_spec`)
   - Suggested direction is a single value `code_gap` or `spec_gap` (exclude `needs_design` and any item whose direction is an "either/or" verdict)
   - Confidence is `high`
   - The correct behavior has an undisputed source: the spec states it explicitly (e.g., an error code table) or an existing correct pattern in the codebase (e.g., the same construct written correctly elsewhere). If confirming "what is correct" requires interpretation → exclude.

B. **Small change scope:**
   - Involves ≤ 3 files, all within the same change domain (all code, or all spec documents) — no cross-domain spec↔code linkage
   - Change type ∈ {fix dead branch / wrong parameter, wire an existing but unconnected call, spec wording correction, add missing test}
   - Exclude: new mechanisms, API / data-structure contract changes, items requiring core changes or exposing a core gap

C. **No user decision needed:**
   - The fix does not change currently-working behavior (it repairs a dead branch or a missing piece that never took effect)
   - No boundary verdict, no "shrink spec vs change code" either/or

Any item failing any criterion → decision group. **P0/P1 items always go to the decision group** — the batch group only contains P2/P3.

==ATOM_BEGIN:batch_findings_mechanism==
**Assertion re-verification:** After eligibility passes, the main agent re-verifies each batch group candidate's core assertion with 1-2 deterministic checks:
- Sub-agent claims "X is missing" → confirm X is really absent from the cited location (read the file, check existence, or grep as appropriate)
- Sub-agent claims "the cited source states X explicitly" → read the cited line, confirm X is really written there, without vague wording ("or", "suggested", alternatives)
- Sub-agent claims "the correct pattern exists elsewhere" → confirm that reference really exists

Re-verification failure → item moves to the decision group. This runs before execution because a post-execution check re-run cannot detect a wrong-direction fix — once the documents are made consistent, the re-run passes and the error is cemented.

**Execution boundary:** Classification is presentation only — it is not an authorization to act. Nothing is implemented until the user explicitly agrees to the whole batch group. The user may approve the batch, release it without changes, or move individual items out to the decision group. Before confirming, the user may ask to expand any batch item (full analysis plus on-the-spot re-check); expanded items move to the decision group and are decided individually.

**Batch group presentation note:** When batch grouping is active, add: "This grouping is a classification suggestion only — nothing is applied until you confirm. Each item shows its judgment basis; ask to expand any item if in doubt — expanded items move to the decision group."
==ATOM_END:batch_findings_mechanism==

### Final synthesis (only when relationships are assigned or findings exist)

When the plan assigns relationships or the primary pass produced findings, one independent read-only final session runs last (generated with `specflowctl gate-mission --run {run_id} --final --format prompt`, recorded with `specflowctl gate-submit --run {run_id} --session cross --keys cross --report PATH`; a run with neither assigned relationships nor findings finalizes directly). The final session checks only the assigned relationships and disposes existing findings; it does not repeat local checks. Its selected names and per-relationship dependency declarations are defined in `framework/verification_scope.md` §Final synthesis. Local PASS results do not remove this work. It consumes the primary sessions' structured findings and the complete source context:

1. Collect all accepted alignment/quality results and every carried judgment.
2. The independent final-synthesis executor may raise each retained finding's canonical severity conservatively per `framework/severity_policy.md` §9 before its synthesis result is accepted:
   - Each finding's severity is assigned by the session that raised it; the final synthesis never lowers it
   - On retain or merge it may raise a finding's canonical severity when the read evidence proves the impact is larger than the graded severity: read at least one target file beyond the surface it was graded on that its impact claim depends on (a caller in a dependency unit, the callee implementation, or the appendix governing the affected behavior), and verify the §9.3 boundary holds against the read evidence
   - Evidence rules (§9.4): raising requires positive evidence read from the target; no evidence → keep the session-assigned severity
   - Deterministic severity mappings (e.g. Step 1 declaration table rows) keep their contract-decided grade
   - The canonical severities drive the derived counts and blocking; they are not restated as a report section
3. `gate-submit` verifies that the final synthesis disposes every input finding exactly once, publishes the complete effective status map and one reasoned `Quality conclusion` for every design and architecture key (including carried keys), and binds every consumed result digest. A non-`ALIGNED` preserve verdict fails its key or is carried by a finding deferred to that protected unit as peer-owned record drift; a preserve finding cannot be suppressed or merged (`framework/verification_scope.md` §Deferred findings)
4. The main agent classifies the retained findings per §Batch classification (skip when the activation threshold is not met); this presentation classification cannot change severity, retention, or the gate result
5. Present the tool-derived consolidated findings summary (§Summary format)
6. Wait for the user's decision per HARD RULE 3a:
   - **Batch group:** one decision on the whole group per §Batch classification.
   - **Decision group:** present each finding with its suggested direction and wait for the user's decision. Do not offer a structured resolution menu.

### Summary format

Present the findings using the unified report skeleton (§Output Format). The header, `Blocking promote`, key counts, and `Next step` follow the skeleton; the Findings section is command-specific and defined here.

```
────────────────────────────────────────────
verify@{unit} · full · candidate | stable   # targeted runs: verify@{unit} · targeted (user requested: {keyword}) · candidate | stable
Result: PASS | FAIL   # P0/P1 → FAIL; all aligned or P2/P3 only → PASS (pending items in counts)
Blocking promote: yes | no   # yes only when P0/P1 findings exist (FAIL)
Key counts: Findings: N (P0: a | P1: b | P2: c | P3: d)   # blocking mismatches = P0/P1 counts; non-blocking mismatches = P2/P3 counts
────────────────────────────────────────────
Findings: none
# or, one entry per finding (the block is rendered from the item's accepted analysis result):
Findings:
  Batch group (N items) — direction resolved, suggested for batch handling:
    - [{severity}] {location} — {issue} (actionable, based on: spec@file:line | code@file:line)
      problem: ...
      evidence:
        - spec: ...
        - code: ...
      impact: ...
      fix: ...
      root_cause: ...
      direction: {spec_gap | code_gap} (confidence: {level})
    - ...
  Decision group (M items) — decided one by one:
    - [{severity}] {location} — {issue} (actionable | needs_decision)
      problem: ...
      evidence:
        - spec: ...
        - code: ...
      impact: ...
      decision: ...
      options:
        - ...
      root_cause: ...
      direction: {needs_design | blocked} (confidence: {level})
    - ...
────────────────────────────────────────────
{body — Items / Scope / Integrity / Coverage / divergence analysis}
────────────────────────────────────────────
Next step: {per direction table — spec_gap → "update the candidate spec, then re-run `verify@{unit}:{keyword}`"; code_gap → "implement the code change, then re-run `verify@{unit}:{keyword}`"; needs_design → "redesign the candidate, then re-run validate and verify"; blocked → "awaiting your decision on {item}"}
────────────────────────────────────────────
```

The resolution label maps from the suggested direction: `spec_gap` / `code_gap` → `actionable`; `needs_design` / `blocked` → `needs_decision`.

When batch grouping is inactive (threshold not met), the Findings section uses the flat format:

```
Findings:
  [{severity}] {location} — {issue} (actionable | needs_decision)
    problem: ...
    evidence:
      - spec: ...
      - code: ...
    impact: ...
    fix: ...   # or decision: ... with options: sub-lines
    root_cause: ...
    direction: {direction} (confidence: {level})
  ...
```

**Batch group presentation note:** The note text is defined in §Batch classification.

### Direction table

| Direction | Meaning | Next step | Promote gate |
|-----------|---------|-----------|:---:|
| **spec_gap** | Code's behavior is correct, spec needs updating | Update candidate spec → suggest re-running validate (affected checks) and verify (affected content); user triggers | P0/P1 → BLOCK; P2/P3 → Allow |
| **code_gap** | Spec's intent is correct, code needs updating | Implement code → suggest re-running verify (affected content); user triggers | P0/P1 → BLOCK; P2/P3 → Allow |
| **needs_design** | Neither side matches a coherent design — needs rethinking | Redesign candidate → suggest validate then verify; user triggers | P0/P1 → BLOCK; P2/P3 → Allow |
| **blocked** | Mismatch depends on external input or unresolved decision | User unblocks → suggest re-running verify; user triggers | P0/P1 → BLOCK; P2/P3 → Allow |

### Fix execution rules

The direction table determines WHO decides (the user, per HARD RULE 3a) and WHAT the next step is, but not HOW to execute the fix. This section governs the fix execution phase — between the user's direction verdict and the re-check. Its purpose is to keep the fix faithful to design intent: the spec is a design document (the system's intended behavior), not an implementation record. These constraints take effect at fix execution time; a re-check can only verify spec-code consistency, never fidelity, so the constraints cannot be deferred to re-check.

**code_gap fixes (change code):**

- The fix baseline is the behavior the spec declares — the code must implement the spec's stated semantics, using the acceptance items / behavior declarations as the reference.
- Do not silently shrink spec semantics to simplify implementation (lowering thresholds, deleting branches, swallowing errors). If a smaller semantic seems warranted by implementation cost, that is a spec change — stop, return to the user for a new verdict (the spec_gap determination path), and do not modify the spec as a bypass.
- The fix addresses only the finding the user ruled on. Do not introduce behavior changes the spec does not declare.

**spec_gap fixes (change spec):**

- Only the candidate layer may be modified (stable cannot be modified directly — reconciliation goes through forking the unit (`specflowctl fork --unit <name>`), see Stable-only mode below).
- Before deleting, rewriting, or weakening a behavior declaration, determine what the declaration is:
  - **Design intent** (the system's intended behavior / external semantics): deletion or rewriting requires a design-intent change basis — an internal spec contradiction, a conflict with a more authoritative source (stable consensus, a bound/global rule, an external protocol), or git history proving the intent was abandoned. "The code happens to behave this way" is not, by itself, a basis.
  - **Mechanism description** (how the behavior is implemented): may evolve with the implementation — but a design intent must not be demoted to a mechanism description to bypass the constraint above.
- The spec keeps its design-document nature: behavior declarations are written as intended behavior (what the system should do), not rewritten as an implementation record (e.g. "truncated at buffer write, no longer truncated at display" style implementation accounting).

**General constraints:**

- Do not claim the fix is verified before it is applied — the existing "fixed, pending re-confirmation" (`fixed_pending_recheck`) rule below applies.
- Fidelity does not depend on re-check: a re-run verifies consistency only, so the constraints above bind at fix execution time.

### Re-check after fixes

Executing quality-gate commands is user-triggered only (see HARD RULE 2 in `framework/concepts.md`). After a fix is applied, the agent must NOT re-run verify automatically. The agent guides the user to a targeted re-check with the concrete command and waits for the user to trigger it:

- **Spec-only fixes** (spec prose, acceptance items, affects fields): map the edits to the affected content — edited body sections → those chapters and their corresponding acceptance items; edited item fields → those items; edited appendix files → the chapters referencing them. Propose `verify@{unit}:{keyword}` for a targeted re-check, or `verify@{unit}` if the user wants complete verification.
- **Code fixes**: propose `verify@{unit}:{keyword}` (keyword = the fixed feature/area) or `verify@{unit}` for complete verification.
- Until a re-check is triggered, do NOT claim the fix is verified — report "fixed, pending re-confirmation" (`fixed_pending_recheck`). A finding confirmed resolved by the re-check is reported as `verified` ("Re-check passed.") — this confirms the fix, not cache freshness. Targeted re-checks never write a cache (`framework/validation_cache.md`); cache freshness is restored only by a user-triggered `verify@{unit}` full run.

After presenting the summary:
- If batch grouping is active: stop and wait for the user's decision on the batch group (approve / release / move items out) and on each decision-group finding. Nothing is implemented without explicit user agreement.
- If batch grouping is inactive: present each finding with its suggested direction and stop. The user decides the next step per HARD RULE 3a. Do not offer a structured resolution menu.

### Stable-only mode

When no candidate spec exists (verify against stable):

```
1. Run Steps 1-6 against stable spec
2. If all items are ALIGNED and every declared code file passes → report PASS; `gate-finalize` writes the verify
   cache with `target: stable` (drift confirmation state consumed by
   `fresh@stable`; `mode: full`, severity counts at 0, `blocking: false`,
   hash + deps evidence — same coverage sequence as Step 8)
3. If any MISMATCH:
   - The implementation has drifted from recorded stable truth
   - Do not suggest spec modifications (cannot modify stable spec directly)
   - `gate-finalize` writes a failure record (`result: fail` + `blocking: true`, `mode: full`, `basis: full`, and the per-item `status` map — `pass`/`fail` for every acceptance item and declared code file; full runs re-execute local checks and may carry unchanged relationship judgments — the confirmation state stays visible as BLOCKED and is the failure-recovery baseline)
   - Report the drift and recommend forking the unit (`specflowctl fork --unit <name>`):
     "Current implementation diverges from stable spec at {details}.
     Recommend creating a candidate round via `specflowctl fork --unit <name>` to reconcile the difference."
```

The stable verify cache is read-only state: it grants no promote eligibility (stable has no gate). Delta re-runs (`reverify`) apply to stable-only targets with a usable baseline — a pass cache (`result: pass`, STALE recovery) or a failure record (failure-record recovery); a MISSING stable cache needs the full confirmation run — see `framework/verification_scope.md` §Stable-only Targets and §Delta Runs → Layer applicability.

---

## Step 8 — Write verify cache (tooling finalize)

After all 7 steps complete, determine cache action based on the highest severity MISMATCH (the finding-level verdict). The suggested direction (spec_gap / code_gap / needs_design / blocked) does not change the gate — blocking is determined by severity only. The cache `result` field uses the gate vocabulary: `pass` means no P0/P1 blocking findings; `fail` is the failure-record state, written by a delta re-run's FAIL and by a candidate full-run FAIL (`mode: full`, `basis: full` — see below and §Delta re-run).

==ATOM_BEGIN:cache_evidence_path_forms==
**Declaring cache evidence — path forms:** spec objects resolved by name other than the run's own target files are declared as **logical references** instead of physical paths — `unit:{name}` for a unit main spec, `unit:{name}:appendix:{file}` for a unit protocol appendix (the full appendix file base name without `.md`, e.g. `unit:auth:appendix:unit_auth_account_token_claims`), `rule:{id}` for a rule file — with the `hash` + `deps` of the file actually read. The run's own target files (the unit's own main spec and appendices; for a rule target, the candidate rule file and its stable sibling) and code files keep physical paths. A logical reference resolves at freshness time to the current-layer file (candidate first, stable fallback), so promoting the referenced unit or rule does not stale a cache whose dependency content is unchanged (see `framework/validation_cache.md` §Logical References).
==ATOM_END:cache_evidence_path_forms==

- **If all ALIGNED:** the coverage run writes the verify cache per `framework/validation_cache.md` format:
  - `gate-finalize` creates `docs/specs/meta/validation/unit/{name}/` as needed
  - Each session report declares its files' dependency scope in its `Dependency scope:` lines (`ranges` / `sections` / `acceptance_items` / `acceptance_item:<id>`); `gate-submit` validates path membership against that session's `read_refs` (not merely the run snapshot), and `gate-finalize` computes the `hash` + `deps` evidence from the accepted reports, tagging each check with its lens. The values come from the accepted reports' `Dependency scope:` lines; a file reported as `all` (or not reported) is declared as a whole file. The declared ranges must cover every region the alignment judgment depended on — including called functions and referenced structures (declare-heavy principle; see `framework/validation_cache.md` §Dependency Declaration). **The unit's own main spec is declared by item regions per item:** each item judgment declares its own `acceptance_item:<id>` region (plus any section regions it read) — the per-item declarations become the cache entry's `checks` mapping (check key = `item:<unit>:<item>`, lens = `alignment`), with the union in `deps` (see `framework/validation_cache.md` §Format → Per-check evidence). A judgment that read the item set as a whole declares `acceptance_items`. All read code files also use exact whole-file fingerprints in their immutable records. Public code checks declare `all`; spec dependencies retain chapter and acceptance-item scope.
  - `gate-finalize` writes `verify_result.md` with `result: pass`, severity counts at 0, `target: candidate` (candidate round) or `target: stable` (stable-only mode), `mode: full`, `blocking: false`, file hashes and dependency CIDs, and the merged `alignment` + `quality` sections

- **If any P0/P1 MISMATCH exists (verify FAIL, full run, candidate target):** the finalize writes a failure record (`result: fail` + `blocking: true`, `mode: full`, `basis: full`; the per-item `status` map — `pass`/`fail` for every acceptance item and every declared code file; full runs re-execute local checks and may carry unchanged relationship judgments — and the file evidence are assembled by `gate-finalize` from the accepted reports) — the failure-recovery baseline. After the findings are resolved, `reverify@{unit}` re-checks the failed keys, persisted `invalidated_checks`, newly affected keys, current keys the baseline never declared, and explicit `--rerun` overrides, then carries the remaining `pass` keys over (see §Delta re-run and `framework/verification_scope.md` §Delta Runs → Failure recovery). This is safe because verify is anchored to the validated spec — carried keys' evidence is unchanged and not contradicted by a targeted P0/P1. Report blocking findings. Agent must stop and not proceed to promote. (A stable-only full FAIL writes the same failure record shape with the per-item status map — see §Stable-only mode.)

- **If all mismatches are P2/P3 (non-blocking, verify PASS):**
  - Collect dependency evidence for every file read during verification (same declaration procedure as above)
  - `gate-finalize` writes `verify_result.md` with `result: pass`, `blocking: false`, severity counts (`p0_count`...`p3_count`), `target: candidate` (candidate round) or `target: stable` (stable-only mode), `mode: full`, file hashes and dependency CIDs
  - Report findings as non-blocking — agent may continue (P2/P3 findings do not block promote).

- **Targeted runs (`:{keyword}`):** report findings only — do not publish a complete cache. PASS leaves cache state unchanged. On P0/P1, immediately run `specflowctl gate-invalidate --gate verify --unit {name} --target {candidate|stable} --check {item id}`: it deletes a pass cache or persists the contradicted item on a failure record and invalidates a matching open run. Blocking findings at any granularity mean promote must not proceed (`framework/validation_cache.md`).

### Delta re-run (reverify@{unit})

Candidate targets, plus stable-only targets with a usable baseline — a delta re-run against a stable-only target whose confirmation cache is MISSING reports the missing-baseline message — "No usable confirmation baseline. Run the full `{command}@{target}` (confirmation check) first, or `specflowctl fork {--unit|--rule} {name}` to start a new round." — and stops (see `framework/verification_scope.md` §Delta Runs → Layer applicability). Triggered by the user when the cache is STALE or BLOCKED (a failure record). Follow §Delta Runs in `framework/verification_scope.md`:

1. **Preconditions** — the existing cache must have `mode: full` and one of the two baselines: `result: pass` (stale-cache recovery) or `result: fail` + `blocking: true` (failure-record recovery, §Failure recovery). A MISSING cache has no baseline — run `verify@{unit}` instead. A failure-record recovery of a full-run FAIL's record is anchored to the validated spec — the recovery's `pass`/`carried` items are trusted only because the unit's validate gate has passed (promote enforces this; see §Failure handling by gate role in `framework/validation_cache.md`).
2. **Plan (mechanism-derived)** — run `specflowctl gate-plan --gate verify --unit {name} --target {candidate|stable} --relationships {names|none} --mode delta` (or `--mode repair` for a failure record). The planner derives the re-run coverage set from the cache's per-check `checks` mapping — affected = {keys whose declared deps went stale} ∪ {current keys the baseline never declared} ∪ {persisted `invalidated_checks`} ∪ {explicit `--rerun` overrides} — and degrades conservatively to the full coverage set when a stale source has no per-check mapping, the cache lacks per-check evidence, or an invalidated key cannot map to the current surface. A targeted P0/P1 is persisted immediately by `gate-invalidate`, so repair must re-run it without any remembered `--rerun` argument. It reports the re-run coverage keys, carried-over keys (`carried_keys`), and any degradation or full-scope coverage statement. The plan is the scope; the executor does not re-derive or extend it.
3. **Execution** — execute the planned coverage keys (protocol: `framework/verification_scope.md` §Coverage model), generating each session's mission with `gate-mission`, submitting each report with `specflowctl gate-submit --run <run_id> --session <session_id> --keys <keys> --report PATH`, and (when relationships are assigned or findings exist) generating the final synthesis with `gate-mission --run <run_id> --final`. Step 7 applies unchanged: a MISMATCH drives the divergence analysis, with the same input table and Context scope fill rule. Any P0/P1 MISMATCH → the finalize writes a **failure record** (`result: fail`, `blocking: true`, severity counts, findings body, `mode: full`, `basis: delta`, per-key `status` map — `fail`/`pass` for the re-run keys, `carried` for the carried-over ones, derived by `gate-finalize` from the accepted reports). Stop, present findings (same as full FAIL).
4. **On PASS** — `gate-finalize` rewrites `verify_result.md` with `result: pass`, `mode: full`, `basis: delta` (or `basis: repair`), `target: candidate` (stable-only target: `target: stable`), `blocking: false`, severity counts, a fresh `timestamp`, and a **complete** `files` list: new `hash` + `deps` + per-check `checks` evidence (computed by the tooling from the accepted reports, tagged with their lens) for the re-run keys' files, and the original evidence for the carried-over keys' files, merged by the tooling from the baseline cache. Logical references (`unit:{name}` / `unit:{name}:appendix:{file}` / `rule:{id}`) are carried over the same way. P2/P3 findings are carried by the severity counts exactly like a full run.

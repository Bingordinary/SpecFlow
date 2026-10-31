# Verify Checklist (both lenses)

## Overview

`verify` is one gate with **two lenses**:

- **`alignment`** — the spec-vs-code check: acceptance items, structural alignment, scope, retirement, surplus, stubs, and divergence analysis (Steps 1–7 below, split across the item sessions and the one unit-level scan session). The spec is **authority**: a divergence is always a finding and is never suppressed.
- **`quality`** — the spec-aware code-quality check: structure/boundaries, naming/abstraction, duplication, coupling, error & safety, hygiene, unplanned debt, architectural quality, and test quality. The spec is **rationale**: a code-quality finding is suppressed when the spec records the design decision behind it.

The **`alignment` lens runs in two session kinds**:

- **item sessions** (`item:<unit>:<item>`) execute **Steps 2, 3, 4, and 6** for their assigned acceptance items, and **Step 7** for the mismatches they raise;
- **one unit-level scan session per run** (`unit:<unit>`) executes **Step 1 (structural alignment)** and **Step 5 (implementation integrity and code design-surface surplus)** over the whole unit, and **Step 7** for its own mismatches.

The unit-wide work is not repeated per item: one scan covers the unit, and the item sessions judge their own items. `framework/verification_scope.md` §Coverage Model owns the key model and the session split.

### Promise-model units

Every unit has `## Promises`; `gate-plan` runs the promise shape, schema,
coverage, dependency, and graph checks before any run state is written. An
invalid promise model — a unit without `## Promises` included — rejects the plan. Item sessions verify the item's
behavior and evidence; they do not re-verify provider code. A passing verify
finalize records `promise_state_<layer>.yaml`, which `fresh` uses for the
`done:` column and the global `DONE` count. See
`framework/core/promise_model.md`.

Item sessions read the item's declared `source` blocks first: they are the
narrative behavior the item proves. Source completeness is a validate check
(5h for promises; 5a for items), not a design-judgment obligation: a design
record's `Spec basis` may name a `##` section or a promise entry for context,
and those are not `source` candidates.

### Run Orientation and Session Plumbing

The trigger route in `framework/concepts.md` loads this file at verify time, not proactively. The judge of every key is an independent read-only subagent; the tooling derives the run's reviewer units from the uncovered coverage set — one per report kind and lens (one per reviewed file for verify quality) — and the main agent joins the units of one lens and kind into a session, splitting a session's key set only when a session declares it exceeds that session's capacity, generating each session's mission with `gate-mission` and recording each report with `gate-submit` (see `framework/verification_scope.md` §Coverage model → §Session capacity).

## Lenses and boundary ownership

**Boundary ownership** — which lens owns which judgment:

| Judgment | Owner | Rule |
|---|---|---|
| Test **quality** (assertion authenticity, mock density, tautology, happy-path-only) | `quality` | Tests are graded as code here. The `alignment` lens uses tests only as behavioral evidence for an item, never as a quality grade. |
| Code within this unit's responsibility with **no spec basis** (surplus) | `alignment` **first** | The `alignment` lens attributes responsibility before judging surplus. Only if the code survives that decision — the code is legitimate and the spec gap is not a drift — does it become a `quality` judgment. |
| **Stubs** | by nature | A stub that violates a *declared* behavior is an `alignment` finding (implemented-but-absent). A stub that is merely poor code (unreachable, placeholder without a declared behavior) is a `quality` finding. |
| **Debt markers** (TODO/FIXME/XXX/HACK comment markers) | `quality` | Dimension 7 owns unplanned-debt markers: a comment marker is a work trace, not a behavioral mismatch. A marker contained in the spec's `known_debt` is suppressed; an undeclared marker is a non-blocking P2/P3 observation. Stubs and placeholders (missing implementation where behavior is declared) remain `alignment` Step 6 findings. |
| **Spec-sanctioned suppression** | `quality` **only** | A recorded spec rationale suppresses a `quality` finding. The `alignment` lens never suppresses a divergence. |
| Step 1 structural alignment and Step 5 implementation integrity and surplus detection over the whole unit | `alignment` — **unit-level scan session** | One scan per run covers the unit and pins its conclusions as the single `unit:<unit>` judgment; item sessions do not re-derive them. Step 5 includes its non-runnable assessment (Part 2), so that check has a session owner too. The scan's report must carry one line per declaration class, so a class it never examined is visible instead of silently missing. |
| Step 2 acceptance alignment, Step 3 scope accuracy, Step 4 retirement verification, Step 6 stub adjudication | `alignment` — **item session** | Judged per assigned acceptance item. Step 3 stays with the item even though Step 1 also compares spec structures to code: scope is a per-item declaration check ("does this item's declared surface carry its behavior?"), while Step 1 is a unit-wide structure check |
| Step 7 divergence analysis | `alignment` — **the session that raises the MISMATCH** (item or unit-level scan) | The reviewer completes the analysis in the same session; see §Step 7 |

### Coverage Keys and Session Partitioning

A session's keys come from **one lens only** — a session never mixes lenses. The two lenses share one P0–P3 severity scale and are merged into the one `verify_result.md` cache, which carries an `alignment` section and a `quality` section.

Coverage keys:

| Lens | Coverage keys |
|---|---|
| `alignment` | `item:<unit>:<item>`, plus one `unit:<unit>` per run (the unit-level scan) |
| `quality` | `code:<file>`, `design:<unit>:<file>`, and one `architecture:<unit>` |

## The quality lens

The `quality` lens's standard — core principle, pre-review setup, review process, the eight dimensions, the P3 reportability gate, and the finding output format — is defined in full below and is delivered into this file by the atom `spec_review_standard`.

==ATOM_BEGIN:spec_review_standard==
## Quality Lens Standard

The `quality` lens separates reusable public code facts from unit-specific design judgments and the unit's overall architecture assessment.

### 1. Core Principle

`code:<file>` records facts and potential problems without a unit-private rationale. `design:<unit>:<file>` treats that unit's spec as rationale: actively check its requirements and retain or exclude every public observation from its assigned file's immutable record with evidence. A unit-specific exclusion never removes a public observation. The alignment lens treats the spec as authority.

### Session composition and batch inputs

Public execution batches do not enlarge the design session's inputs. A co-batched session holds a file's `code:<file>` key and its unit's `design:<unit>:<file>` key together: the same reviewer collects the facts and then judges the unit design against them in one pass — the facts stay rationale-free and publish as the file's public record, so other units can still reuse them. `architecture:<unit>` assesses Dimension 8 once for the entire unit.

### 2. Pre-review Setup

Public checks read the file evidence surface fixed by the mission: the reviewed file, its cited `affects.evidence_files`, the declared dependency spec objects and applicable public rules, plus the related files the coordinator supplied through the input manifest. Related evidence is never auto-linked by filename or stem. Do not read unfinished peer designs. A missing evidence path is reported (`Verification could not complete — missing read ref: <repo-relative path>`), not judged around; the coordinator adds it to the open run with `specflowctl gate-extend`.

Design and architecture checks read their selected unit spec, applicable rules and code. Extract accepted trade-offs, architectural decisions, design constraints, known debt and non-goals from that unit's published design context.

### 3. Review Process

1. Actively check the unit's spec requirements, including violations not present in the public record. Report new findings independently.
2. Assess the six Dimension 8 fields in the unit's architecture task once.
3. Public potential problems are observations rather than gate-driving findings. Unit design findings drive this unit's gate. The standard severity and finding format below applies to their presentation.

### Session inputs and record reuse

1. Reuse accepted public observations only when coverage, code content and protocol are valid; otherwise execute the public check.
2. Consume the public records for the design session's assigned files only, whether executed, reused or carried. For each observation in those records, the design reviewer reports `Observation disposition: <id> = retained|suppressed — <unit-specific evidence and reason>`. In a co-batched session the design reviewer disposes the observations collected in the same report's code blocks instead of consuming a published record: write the code blocks first, then reference each observation by its tool-assigned id — finding ids are `<run>/<session>/F<k>` in report order across the whole report, not restarted per file.

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
  - The unit reaches past the promise `depends_on` boundary and reaches into a dependency unit's internal implementation → P1
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

P0/P1 findings block promote.

#### Promote gate and cache requirement

The promote gate additionally requires a cache written by a full run — targeted re-checks do not write a cache, so they never satisfy the gate. The `quality` lens and the `alignment` lens share the same P0–P3 scale; their graded findings are merged into the one `verify_result.md` cache.

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

Public and design sessions contain exactly one `File: {assigned check key}` block per assigned file. Public blocks contain `conclusion: FACTS`, `facts` and potential observations. Design blocks contain a quality conclusion, `spec_requirements`, one or more `Spec basis` anchors, `gate_findings`, an evidence-backed disposition for each public observation, and new findings (`gate_findings` is `none` or one or more `[P0|P1] {finding}` entries separated by `;`; a retained P2/P3 observation is recorded by its own finding block plus its `Observation disposition`, never in `gate_findings`). A design block's `Spec basis` lines name, one anchor per line, the spec content the judgment relied on — a promise entry id, or the heading text of a `##` section or of a `###` or deeper narrative block, where heading text is the heading line without its leading `#` markers and surrounding space (the heading `### 3. Data Structures` is named `3. Data Structures`) — and the tooling rejects a report whose anchor does not resolve; the record's spec pins are derived from exactly those anchors, so an edit to a declared anchor re-runs the judgment while an edit to narrative it never declared does not (`framework/verification_scope.md` §Verify scope). Each finding block is written inside the `File:`/`Unit:` block of the file or unit it judges — after that block's content line (`facts:` for a public block, `gate_findings:` for a design or architecture block) and before the next `File:`/`Unit:` line — because a finding binds to the block it appears in; a finding whose `location` names a file this session also judges must be placed in that file's block, and the tooling rejects a report that binds it elsewhere (listing finding blocks after the last file block binds them to the last file's key). Architecture uses one `Unit: architecture:{unit}` block, the six Dimension 8 assessments, conclusion, `gate_findings`, and `Suppressed by spec (N)`. Reports carry no dependency-scope lines: the tooling records the run's whole input surface. Batching never shares one subject's verdict or findings with another.

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
==ATOM_END:spec_review_standard==

## The alignment lens

The `alignment` lens is the remainder of this file (Steps 1–7 and the analysis collection). It treats the spec as authority and never suppresses a divergence.

The steps are split by session: **Step 1**, **Step 5**, and their **Step 7** analyses run in the unit-level scan session (`unit:<unit>`); **Steps 2, 3, 4, and 6**, and their **Step 7** analyses, run in the item sessions (`item:<unit>:<item>`). Each step below states its session.

## Target-layer applicability

Use the planned `Target layer` (`candidate` or `stable`) shown in the gate mission when applying this checklist. For a targeted run without a gate mission, use the layer resolved by the command: candidate when present, otherwise stable. The Read Procedure resolves the main spec and the appendices from that layer, and every step reads that layer's content. A stable-only target is a drift confirmation: run Steps 1–6 against the stable content, do not enter divergence resolution (Step 7), and do not propose spec or code repairs — report a MISMATCH as drift and recommend forking the unit (see §Stable-only mode).

## Prerequisite — Read all unit files

The unit's complete spec is the union of the main spec and all appendix files. All verify steps that follow operate on this union. The `## Promises` block is the formal contract; appendix content is explanatory support truth — examples, detail, and evidence — not a second contract. Appendix claims are verified as they support a promise, and normative contract content that appears only in an appendix is a validate finding (Check 5a step 6), not a verify contract.

### Read Procedure

Before executing any verify step, the agent must read the complete unit spec. `{layer}` is the resolved target layer (`candidate` or `stable`) per §Target-layer applicability:

1. Read the main spec: `docs/specs/units/{layer}/unit_{unit}.md`
2. Glob all appendix files in the resolved layer: `docs/specs/units/{layer}/appendix/unit_{unit}_*.md`
3. For each appendix, read its frontmatter and confirm ownership by the appendix `unit` field.
4. Read the content of every appendix file.

## Mode Selection

| Trigger | Mode | What to execute |
|---------|------|-----------------|
| `verify@{unit}` | full | Verify all spec content and the declared code surface: the `alignment` lens (item sessions run Steps 2, 3, 4, and 6 per acceptance item, with Step 7 for the mismatches they raise; one unit-level scan session runs Steps 1 and 5 over the whole unit, with Step 7 for its own mismatches) and the `quality` lens (public code and unit design per file, plus one unit architecture task). Sessions share one lens; a quality session may pair a file's code and design keys. |
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
| Test absence (coverage completeness) | No verdict change — recorded as an annotation |

Fold order: MISMATCH > CANNOT_DETERMINE > ALIGNED. A claim supported only by the existence of a test is not deterministic evidence — tests are annotations, not proof of implementation behavior.

After completing all analysis steps, the agent must report coverage confidence (see §Output Format Coverage).

## Target Selection

- If a candidate spec exists → verify code against **candidate** (the current working proposal). Mismatches trigger first-principles divergence analysis (Step 7).
- If no candidate exists but a stable spec does → verify code against **stable** (check if current implementation still conforms to recorded truth). Do not enter divergence resolution — instead, recommend forking the unit (`specflowctl fork --unit <name>`).

## Execution Rules

- Each verifiable claim in the spec reports **ALIGNED** / **MISMATCH** / **CANNOT_DETERMINE** with code references.
- Evidence is always code-level (file:line, struct/function signatures, grep results) — never runtime output.
- A symbol name, comment, test function name, or interface existence alone is not evidence of behavior — read the code that implements the behavior (e.g. an enum value is confirmed by its definition and JSON tag, not by its identifier).
- For CANNOT_DETERMINE claims (e.g., pass_condition requires runtime verification): record the gap and continue.

### Subagent Launch and Permissions

- **Subagent permissions:** may inspect file content, search text by pattern, locate files by name pattern, query read-only repository history (e.g., `git log` for file timestamps), and run the read-only framework inspection subcommands its checks instruct it to run (`specflowctl deps`, `specflowctl surfaces`, `specflowctl validate candidate`). It must NOT modify files, run state-changing commands, or delegate to other agents. The main agent launches one read-only sub-agent per session (each session covers a computed, lens-pure batch of coverage keys) — each sub-agent follows the same permissions (read-only, no delegation chain).

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
- `{mode}` — `full` for full runs; `targeted (user requested: {keyword})` for targeted runs; `delta` for a delta re-run (`revalidate@{unit}` / `reverify@{unit}`). A validate delta that finds an own-content change is recorded as a full run (`basis: full`); validate has no repair mode.
- `{layer}` — the spec layer checked: `candidate` | `stable`.
- `Result` — `PASS | FAIL` for both gates. For coverage runs this value is generated by `gate-finalize` from the accepted sessions (or from the single rule-validate session); the coordinator never supplies it. The gate is decided by severity: FAIL means gate-driving retained P0/P1 findings exist. Unit validate grades P0-P3, where P2/P3 are non-blocking advisories (debt and polish recorded in the cache, never gate-driving), so a validate FAIL is always driven by P0/P1 findings; rule validate grades findings P0/P1 only; verify FAIL means gate-driving retained P0/P1 mismatches/findings exist. A finding deferred to another unit is retained and routed but is not gate-driving — it never causes FAIL.
- `Blocking promote: yes | no` — `yes` when the run FAILs (gate-driving P0/P1 findings exist); `no` otherwise. Valid for both gates.
- `Key counts: Findings: N (P0: a | P1: b | P2: c | P3: d)` — N is the gate-driving retained finding count and a/b/c/d are generated by `gate-finalize`; the coordinator never supplies them. Unit validate's P0/P1 counts are gate-driving and its P2/P3 counts are recorded non-blocking advisories (`framework/validation_cache.md` §Advisory carry); rule validate grades findings P0/P1 only, so its P2/P3 counts are always 0; verify's blocking mismatches equal the P0/P1 counts and non-blocking mismatches equal the P2/P3 counts. A finding deferred to another unit is excluded from these counts (it is presented under `Deferred to {unit}:`). Gate-specific summary numbers (validate's failed checks and advisory findings, verify's coverage, the quality lens's suppressed-by-spec count) appear in the body.
- `{body}` — gate-specific content defined in this file's body format section (validate: one line per check; verify `alignment`: per-item alignment and divergence analysis; verify `quality`: architecture assessment and suppressed findings).
- `Findings:` — `Findings: none` when no finding is retained; otherwise one entry per finding: a unified entry line `[{severity}] {location} — {issue} (actionable | needs_decision)` followed by the finding block's required detail lines. `actionable` — a concrete repair can be made without user judgment (alignment: direction spec_gap/code_gap; quality: a determined recommendation); `needs_decision` — requires user input or a design decision before the fix can be made (alignment: direction needs_design/blocked; quality: architecture trade-offs). The block is written for a reader who has not opened the spec or the code: delete every `ref:` line and every `§`, line-number, or other anchor reference from the field text, and the reader must still learn what is wrong, what the conflicting sides say, what is at risk, and what to fix or decide. Anchors appear only in the optional trailing `ref:` line, which carries tracking information and no meaning.
  - `problem:` — one sentence naming the concrete subject (acceptance item id, field or parameter name, behavior) and what is wrong with it; never an anchor, region name, or pointer.
  - `evidence:` — the quoted content that proves the claim, as one or more indented `- {verbatim quoted source content} — {source label}` sub-lines. Contradiction findings quote both conflicting sides verbatim; undefined or missing behavior lists the defined part and the missing part; spec-vs-code findings quote the declared behavior and the implemented behavior. A quality P3 finding uses its `fact_anchor:` line as this evidence form.
  - `impact:` — what goes wrong, or stays undecidable, if the finding is not resolved.
  - `fix:` — actionable findings: the concrete repair action. `decision:` — needs_decision findings: the question the user must answer, with `options:` sub-lines listing the candidate choices.
  - The block's detail lines are contiguous indented lines directly under the entry line — no blank lines inside the block (the block is stored and re-rendered verbatim when a finding is carried into a partial run).
  - `{gate-specific detail lines}` — gate-specific extras follow the shared fields: alignment `root_cause:`, `direction:`, and `confidence:`; quality `spec_context:` and its mechanically required `fact_anchor:` for P3 findings; unit validate its mechanically required `fact_anchor:` for P3 findings. Rule validate adds none.
  - Findings are grouped into the batch group and decision group defined in this file's batch classification section when this file defines one; flat when this file defines no batch classification or grouping is inactive. Batch-group entries carry the same fields; terse one-line values are fine.
  - Findings whose recorded ownership routes them to another unit are presented in a `Deferred to {unit}:` section after the Findings section, with the same block fields plus an `ownership:` line. They are retained and routed but do not count toward `Key counts`, `Blocking promote`, or the gate result (see `framework/unit_verify_checklist.md` §Output Format → Deferred findings).
- `Evidence recording:` — reports carry no dependency declarations. `gate-finalize` records the run's whole input surface: one cache entry per input-surface file with its whole-file `hash` and ordered chunk sequence, plus a per-check marker (`check` + lens, and the failure status on failure records) for every executed judgment. Spec objects resolved by name are recorded as logical references so a layer move does not stale the cache (see `framework/validation_cache.md` §Format → Entries). Partial runs record the re-run keys' markers alongside the carried keys' snapshotted markers; targeted runs write no cache.
- `Next step:` — the concrete command to run next with its reason; `None` when nothing further is needed. A finding's fix lifecycle has three states with fixed wording: `finding_open` → "Resolve the findings, then re-run the target-appropriate re-check command (`validate@{target}:check-{n}`; unit targets also `verify@{target}:{keyword}`) to confirm"; `fixed_pending_recheck` → "Fixes applied; re-run the target-appropriate re-check command to confirm." — only after the approved fix was actually written; `verified` → "Re-check passed." — only after a re-check confirmed the fix. A gate report is always produced before any fix is applied (nothing is implemented before the user approves the findings), so an actionable finding's report-time `Next step` is always the `finding_open` wording. Other guidance: both gates green → "if the design is finalized, run `promote@{target}`"; needs_decision → "awaiting your decision on {item}"; nothing further → `None`.

**Targeted runs:** end the report with the gate's targeted note ("This was a targeted check — no complete cache was written. Run `{gate}@{target}` for a complete ...") after the `Next step` line. If the result contains P0/P1, run `gate-invalidate` before reporting completion.

### Completion — Persist Gate Cache

A quality-gate run is complete only when its result is persisted and visible to `fresh`/`promote`. A report alone does not satisfy the gate.

- **Full (`validate@{target}` / `verify@{unit}`, `mode: full`, `basis: full`)** — complete only when the coverage run finished and its cache was written: (1) `specflowctl gate-plan --gate {validate|verify} (--unit {name} | --rule {id}) --target {candidate|stable} [--relationships NAMES|none] [--inputs-file PATH]` fixed the immutable input snapshot, local coverage set, and relationship scope (`--relationships` is required for full unit runs and optional for delta/repair; select names or `none`) **before any executor read input** (before mission assembly / sub-agent launch) — the input manifest adds evidence that every session may read but never creates a coverage key, and it must not be a repository-content file (keep it under the ignored `meta/plan_inputs/` or outside the repository); (2) the tooling's computed reviewer units for the uncovered coverage were joined by the coordinator into sessions — one per report kind and lens (one per reviewed file for verify quality), and a session's key set is split only when a session declares it exceeds that session's capacity — reusing accepted public tasks and waiting for assigned tasks and, for each session, generated a mission with `specflowctl gate-mission --run <run_id> --keys <k1,k2,...> --format prompt`, sent it verbatim to one independent reviewer, and recorded its report with `specflowctl gate-submit --run <run_id> --session <session_id> --keys <k1,k2,...> --report PATH` until `gate-status` reports full coverage; (3) when the run requires a final synthesis — unit validate: assigned relationships or blocking P0/P1 findings only (its P2/P3 advisories need no session); verify: assigned relationships or any finding — one independent final synthesis was generated with `specflowctl gate-mission --run <run_id> --final --format prompt` and recorded with `specflowctl gate-submit --run <run_id> --session cross --keys cross --report PATH`; rule targets never synthesize (a rule with blocking findings writes the failure record directly) and always skip this step; a run with neither assigned relationships nor such findings skips this step; (4) the coordinator ran `specflowctl gate-finalize --run <run_id>` and the cache `docs/specs/meta/validation/{unit|rule}/{name}/validate_result.md` | `verify_result.md` was written (the merged verify cache carries its `alignment` and `quality` sections); (5) `fresh@{target}` (`fresh --unit {name}` / `fresh --rule {id}`) shows the expected gate state (`FRESH` for `result: pass` / `blocking: false`, `BLOCKED` for `result: fail` / `blocking: true`). A candidate **full-run** FAIL writes a failure record (`result: fail` / `blocking: true`; validate/verify records declare `pass`/`fail` for every executed judgment) — the failure-recovery baseline. A finalize that finds mid-run content drift on a full run salvages the run: the result is persisted with the drift's affected report keys recorded in `invalidated_checks` — a salvaged pass cache is a recovery baseline that `fresh` reports `STALE` until its recovery plan re-runs the invalidated keys (validate or rule: full scope; verify: repair); a salvaged failure record keeps its blocking findings (`BLOCKED`, or `STALE` with the findings named when its evidence also changed), and the next plan re-runs the invalidated keys with the failed judgments. A structural divergence (the input surface itself changed) and any drift on a partial run are rejected and write no cache. See `framework/validation_cache.md` §Write Rules and §Failure handling by gate role.
- **Delta (`revalidate@{unit}` / `reverify@{unit}`, `mode: full`, `basis: delta` or `basis: full`)** — the same coverage-run sequence. A **validate** delta is planned mechanically: an external-only change (the unit's own spec union is unchanged; only a provider promise or a rule moved) runs the `external` group and carries the rest; any own-content change, failure record, salvaged cache, moved judging standard, or unusable baseline runs all four groups and is recorded as a full run (`basis: full`). There is no review session and no repair mode for validate. For unit validate, the final synthesis runs when relationships are assigned (the affected standing relationships for an external delta, or the baseline's standing relationships for a full-scope plan) or blocking P0/P1 findings exist; a P2/P3-only run presents its findings directly. A **verify** delta derives the re-run set from each conclusion's own record pins and from the evidence its key currently requires (a new file in a declared surface), adds current keys the baseline never declared and explicit `--rerun` overrides (repair also forces failed judgments, persisted invalidations, and failed relationships), and carries the rest; the whole derived set executes under the standard checklist, and `gate-finalize` merges the re-run evidence with the carried baseline evidence. Rule targets have no delta.
- **Targeted (`:check-{n}` / `:{keyword}`, no complete cache)** — complete when the report is produced and, for P0/P1, the coordinator has run `specflowctl gate-invalidate` for every affected check key. Targeted runs intentionally do NOT create a gate run — `gate-plan` / `gate-mission` / `gate-submit` / `gate-finalize` are not run. A targeted PASS leaves cache state unchanged. A targeted P0/P1 deletes a pass cache or persists `invalidated_checks` on a failure record and invalidates any matching open run. For verify, the same locked transition invalidates the contradicted immutable evidence, so consuming caches and open runs must recheck the affected judgments. It never publishes a complete result cache or satisfies the promote gate. The report ends with `This was a targeted check — no complete cache was written. Run {gate}@{target} for a complete ...`.

**Self-check:** `ls docs/specs/meta/validation/{unit|rule}/{name}/` and `fresh@{target}` (unit: `fresh --unit {name}`, rule: `fresh --rule {id}`).
==ATOM_END:report_skeleton==

### Body format (verify)

The `[P0|P1|P2|P3]` on MISMATCH lines is the severity assigned by the session that raises the finding and carried in the MISMATCH block's own `Severity:` line. Deterministic Step 1 table rows keep their contract-decided grade; the final synthesis may only raise a retained finding's canonical severity conservatively — never lower it (see `framework/severity_policy.md` §9).

#### Lens Body Skeletons

`alignment` lens — **item session** body (`item:<unit>:<item>`, Steps 2, 3, 4, and 6, with Step 7 for the session's mismatches):

```
Items:
  - {item.id}: ALIGNED | MISMATCH (type) [P0|P1|P2|P3] | CANNOT_DETERMINE — code references
Scope: PASS | FAIL — findings
Coverage:
  - items_with_deterministic_evidence: N/M
  - items_reading_only: N
First-principles divergence analysis: (only if any MISMATCH)
  - {item.id}: {suggested direction} [P0|P1|P2|P3] — {rationale}
    User verdict: ...
    Next step: ...
```

`alignment` lens — **unit-level scan** body (`unit:<unit>`, Steps 1 and 5, with Step 7 for the scan's mismatches). The verdict line is the scan's overall conclusion; the declaration lines are mandatory and are mechanically validated, so a class the scan never examined is a rejected report rather than a silent gap. Each retained mismatch carries its own finding block in the same session, in report order — one block per Step 1 declaration-class or Step 5 surplus mismatch, so the scan is not limited to a single finding — with a MISMATCH type restricted to `structural | surplus` (an item session types its own mismatches `structural | acceptance | scope | stub` — Step 4 retirement is structural, Step 5 surplus stays with this scan):

```
unit:{unit}: ALIGNED | MISMATCH (structural|surplus) | CANNOT_DETERMINE — reason
  # on a MISMATCH, indented directly under the verdict line: one finding block per
  # retained mismatch (Problem: ... Confidence:), in report order
declarations.api_endpoints: checked=<n> — result
declarations.function_signatures: checked=<n> — result
declarations.data_structures: checked=<n> — result
declarations.enums_constants: checked=<n> — result
declarations.error_types: checked=<n> — result
declarations.error_messages: checked=<n> — result
declarations.state_machines: checked=<n> — result
declarations.configuration_keys: checked=<n> — result
surplus: found=<n> — result
non_runnable: count=<n> — result
```

`checked=<n>` is how many declarations of that class were located and compared, so `checked=0` states a class that carries nothing (or that the session did not examine — say which, with the reason). `surplus: found=<n>` is the number of surplus candidates that survived Step 5's design-decision filter (Part A). `non_runnable: count=<n>` is the number of acceptance items marked `runnable: no`, and its result states whether each carries a substantive `not_runnable_reason` with external evidence and whether the remaining runnable set gives meaningful coverage (Step 5 Part 2); the line is mandatory, so neither the declaration scan nor either half of Step 5 can be silently skipped. On a MISMATCH, follow the verdict line with one finding block (`Problem:` onward, the format in §Step 7 and §Output Format) per retained mismatch, in report order — a scan that retains several step-classes' or surplus mismatches writes several blocks.

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

---

## Step 1 — Structural alignment

**Session:** the unit-level scan session (`unit:<unit>`), together with Step 5. Item sessions do not re-derive it.

**Purpose:** Cross-reference every static structure declared in the spec body and appendices against the actual implementation code. This catches missing interfaces, wrong signatures, and inconsistent data definitions that acceptance items alone might miss.

**Why this exists:** Acceptance items describe "what to verify" but the spec (body + appendices) describes "how it's built." If a protocol definition or data structure in the spec differs from what's implemented, verify must catch it — even if the acceptance items pass.

**Execution steps:**

1. Read the full spec body AND all appendix files (the design narrative, the `## Promises` contract block, and appendix examples/supporting detail)
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

4. **Reverse direction:** code→spec surplus discovery is owned by Step 5 Part A (the design-surface scan with its implementation-detail filter). Do not run a second structure-level reverse scan here — Step 5 Part A reads the complete associated files, attributes structures to responsibilities (the attribution rules there cover shared files), and defers candidates to Step 7. The subset-match exception from step 3 stands on its own: subset-match declarations whose spec-declared fields are all present in code are ALIGNED (extra code fields are not surplus).

**PASS (ALIGNED):** All spec body declarations have structurally consistent implementations

**FAIL (MISMATCH):** Structural differences found between spec and code — defer classification to Step 7

**Check method:** Spec body × implementation code — structural cross-reference in the spec→code direction (the code→spec direction is Step 5 Part A)

---

## Step 2 — Acceptance alignment

**Session:** item sessions (`item:<unit>:<item>`).

**Purpose:** For each acceptance item, verify that the code structurally satisfies the pass_condition using static analysis. This is not about running tests — it is about confirming the code contains the structures, functions, and patterns needed to satisfy the condition. The `## Promises` block is the complete formal contract surface; Step 1 verifies its declarations against code. Appendix content is support truth for a promise, not a second contract: normative contract content belongs to a promise (validate Check 5a step 6 and Check 5h enforce this).

**Why this is achievable:** A pass_condition like "Returns HTTP 201 with id and created_at" can be verified by checking the handler code returns 201 status and includes those fields in the response struct. The subagent cannot verify the code *works correctly*, but it can verify the code *is structured correctly*.

**Execution steps:**

For each acceptance item in the target spec (appendix content is read as the item's context, not as an independent scan object — validate Check 5a requires normative appendix contract content to be carried by a promise, so the promise block is the complete contract surface and appendix claims are verified through the promise and the items that prove it):

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

**PASS (ALIGNED):** All items have structurally consistent implementations

**FAIL (MISMATCH):** One or more items have structural inconsistencies — defer classification to Step 7

**CANNOT_DETERMINE:** Items that require runtime verification — note them for human review

**Check method:** Acceptance item pass_condition × implementation code — assertion-level structural cross-reference

**Test design sub-check (for verification_type: testable items only):**

Ownership note: the `alignment` lens records whether tests exist for an item (coverage completeness, as annotation evidence) and verifies the evidence-carry check for cited `affects.evidence_files`. The **quality** of the unit's own test files — behavioral vs implementation-coupled, assertion meaningfulness, failure-mode coverage, brittle duplication — is owned by the `quality` lens (see §Lenses and boundary ownership and the quality-lens standard above). A test-quality concern is never an `alignment` finding.

This sub-check identifies significantly implied but missing test scenarios; the alignment reviewer reports them per acceptance item.

**Language-agnostic approach:** The agent reads test files and self-identifies the testing framework (mock libraries, assertion libraries, test runner conventions) rather than relying on a hardcoded language list. When a test framework or assertion style is unfamiliar, the agent reports CANNOT_DETERMINE rather than guessing.

### Per-Item Report Format and Test Discovery

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

For a coverage run, the coordinator locates relevant test and code-context file paths before `gate-plan`, without assessing their behavior, and lists every discovered file in the input manifest (`--inputs-file`). Include files already in the declared implementation surface so delta/repair can detect evidence missing from the baseline. This discovery covers every acceptance item in full, delta, and repair modes. The independent alignment reviewer reads and declares the relevant tests from its session `read_refs`; a required test or context file missing from those refs is an incomplete session, not evidence that no test exists. The coordinator recovers it by extending the open run with `specflowctl gate-extend` — accepted keys keep their verdicts — not by replanning.

---

### Part A — Coverage completeness

Identify significantly implied but missing test scenarios:

1. **Happy path:** Does a test exist that exercises the primary success scenario? If the pass_condition describes a success outcome and no test covers it → CONCERN (possible untested core behavior)
2. **Input variants, business rules, dependency failure:** Does the description or pass_condition strongly imply a scenario (e.g. "register" implies "email already exists", "create order" implies "invalid product ID") that has no corresponding test? If a reasonable developer would expect a test for that scenario and none exists → CONCERN

Reference: `framework/test_decomposition_standard.md` provides decomposition methodology for deeper scenario discovery but is not required reading for this sub-check.

Part A is **not** an exhaustive coverage audit. It flags obvious omissions. A single acceptance item may produce zero, one, or several test scenarios depending on its content. If the implementation is in a language or framework where tests are not written in the expected location, report CANNOT_DETERMINE.

---

### Reporting

The coverage-completeness concern is recorded under the item in the verify output as an annotation. It does not change the item-level verdict (ALIGNED / MISMATCH / CANNOT_DETERMINE).

**Edge case:** If acceptance item has no tests at all, the sub-check flags the missing scenarios.

**Evidence-carry check (items citing `affects.evidence_files`):**

An item may cite read-only evidence files — typically tests or code owned by another unit — in `affects.evidence_files`. For each cited file, the item judgment verifies that the file actually carries the behavior or assertion the item claims: locate the referenced behavior in the cited file and confirm it matches the item's `description` or `pass_condition` at the claimed location. A citation that does not resolve to the claimed behavior is a MISMATCH of this item (root cause and direction per Step 7). The cited file's own quality is never judged here — it is another unit's implementation surface, reviewed by its owner's gates — and an evidence file never enters this unit's quality coverage keys. The unit's own test files keep their quality-lens grading.

#### Annotation Output Format

```
{item.id}: ALIGNED
  - evidence: grep -n "201" src/api/user.go → line 42
  - deterministic: true
  Part A: No concerns
```

---

## Step 3 — Scope accuracy

**Session:** item sessions. Scope is a per-item declaration check and stays with the item even though Step 1 compares spec structures to code.

**Purpose:** Cross-reference each acceptance item's declared implementation files — `implementation_surface` (a file, or a directory expanded to its repository-content files) and `affects.files` — against the actual implementation. This catches undeclared scope and missing declarations.

**Execution steps:**

1. For each acceptance item's declared implementation files — its `implementation_surface` value (a file, or a directory expanded to its repository-content files) plus every `affects.files` entry:
```
- Read each declared file
- Does the file contain implementation relevant to the pass_condition?
- Is the nature of the change consistent with the behavior described?
  (If item says "add login handler" but the file only has imports → flag)
- Does this file participate in the declared behavior? Shared implementation is permitted;
  check this unit's requirements independently
  (see `framework/shared_judgments.md`).
```

2. For each acceptance item with `affects.rules`:
```
- Search for each declared rule name or pattern in the implementation files
- Is the rule being used? How?
- If the rule is not found in the implementation → flag (possible undeclared scope)
```

3. For the promise this acceptance item verifies (its `promise` value): for each entry in that promise's `depends_on`, search the implementation for use of the provider contract; if the declared dependency is unused, flag it. A `non_promise` item has no promise dependency and skips this step.

4. Cross-reference: compare the declared implementation files (`implementation_surface` expansion plus `affects.files`) against the set of files that contain actual implementation related to this acceptance item:
```
- Files with relevant code but not declared → flag (under-declared scope)
- Declared files with no relevant code → flag (over-declared scope)
```

**Scope boundary for non-implementation files:**
- Test files are not scope violations: the declared implementation files (`implementation_surface`/`affects.files`) declare implementation scope; a relevant test file absent from them is context evidence, never an under-declared-scope finding.
- Dependency files (read for context but not part of the implementation) are likewise context evidence, never scope findings.

An under-declared file is a declaration defect, not a read-ref gap: report it as a scope finding whose fix updates the candidate spec's declaration, then re-plan verify.

**PASS:** All declared implementation files are accurate and complete

**FAIL (scope MISMATCH):** Undeclared scope or inaccurate declarations found — defer classification to Step 7

**Check method:** declared implementation files (`implementation_surface` × `affects.files`) × actual implementation — triple cross-reference (files, rules, dependencies)

### Gate-Extend and Input Manifest

`gate-extend` does not substitute — it broadens what a session may read, not what the spec declares (framework/shared_judgments.md §Required tasks). Related evidence the reviewer needs to read but that is not implementation participation goes through the input manifest or `gate-extend` instead.

---

## Step 4 — Retirement verification

**Session:** item sessions.

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

**Session:** the unit-level scan session (`unit:<unit>`), together with Step 1.

**Purpose:** Discover code designs within this unit's responsibility that are not declared in its spec (code surplus) and assess non-runnable items. Do not classify surplus findings yet — defer to Step 7. The reverse-check responsibility boundary from Step 1 also applies here.

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
- Establish whether it implements or constrains this unit's responsibility.
- Record evidence and exclude independent responsibilities outside that scope.
  Another unit sharing the file is not, by itself, proof of exclusion.
- If evidence is missing, add it to the open run with `specflowctl gate-extend` before judging surplus.

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

The assessment's result is reported on the session's mandatory `non_runnable: count=<n> — result` line (§Lens Body Skeletons — unit-level scan body): the count is the runnable: no items, and the result condenses the reasoning concerns above (missing or self-attested `not_runnable_reason`, insufficient runnable coverage). The line is required even when `count=0`, so Step 5 Part 2 always has a visible session result.

**PASS:** No surplus design found; non-runnable items have documented reasons

**FAIL (MISMATCH):** Surplus design found — defer classification to Step 7

**Quality concern:** All or most items are non-runnable; runnable coverage is insufficient

**Quality concern (reasoning):** One or more non-runnable items are missing not_runnable_reason or lack external evidence

**Check method:** Implementation code design surface × spec body — reverse cross-reference (code-to-spec design direction)

---

## Step 6 — Stub & Placeholder Scan

**Session:** item sessions (the plan-time candidate list is attached to their missions).

**Purpose:** Ensure no incomplete implementation hides behind a structurally complete surface. Debt markers (TODO/FIXME/XXX/HACK) are NOT scanned here — they are owned by the `quality` lens (Dimension 7).

**Execution steps:**

1. **Spec-side placeholder check:** For each acceptance item, read the `implementation_surface` value:
   - Value is `<pending>` → MISMATCH: the design-first placeholder was never backfilled (do not classify yet — defer to Step 7)
   - Value is not `<pending>` but resolves to no file → MISMATCH: the declared mapping points at no implementation (do not classify yet — defer to Step 7). `gate-plan` already rejects such a spec, so this state means the surface changed after planning — the finalize snapshot comparison reports the divergence as well

2. **Adjudicate the tool's candidates:** Classify each hit:
   - **RELEVANT** — a real stub or placeholder in implementation code → MISMATCH (do not classify yet — defer to Step 7)
   - **IRRELEVANT** — an idiomatic construct, e.g. Go `return nil`, error sentinels, test fixtures

3. **Residue:** a stub pattern you encounter while reading a file that the scan did not list (the scan covers the declared surface only) is reported the same way: record the per-file result and classify it.

**PASS:** No RELEVANT stubs or placeholders found

**FAIL (MISMATCH):** One or more files contain a RELEVANT stub — defer classification to Step 7

**Check method:** Tool-performed deterministic scan (plan-time) + reviewer adjudication of the candidates

### Plan-Time Scan and Report Format

The scan is deterministic and **tool-performed**: `gate-plan` scans the declared code surface with the fixed stub patterns and records candidate hits in the run's plan notice; item missions carry the hit list in their session context. The reviewer classifies the tool's candidates — it does not grep by hand.

The plan notice lists every candidate hit as `{file}:{line}: {text}` over the declared code surface (patterns: `return null`, `return []`, `placeholder`, `not…implement`, `return Response.json({})`, `w.WriteHeader(204)`).

Per-file result format:

```
{file}: CLEAN | STUB_FOUND
  - line 12: return Response.json({}) (empty_response)
```

If no fresh plan notice is available in the session context (e.g. a rerun under changed state), the fixed commands remain the fallback:

```bash
grep -n "return null\|return \[\]\|\bplaceholder\b\|not.*implement" <file>
grep -n "return Response.json({})\|w.WriteHeader(204)" <file>
```

---

## Step 7 — First-principles divergence analysis

**Session:** the session that raises the MISMATCH — an item session for its Steps 2, 3, 4 and 6 mismatches, and the unit-level scan session for its Steps 1 and 5 mismatches. The reviewer completes Step 7 in the same session.

**Purpose:** When Steps 1-6 detect mismatches, determine the root cause and correct direction using first-principles reasoning — not presence-based heuristics. The independent reviewer completes this analysis before submitting its alignment report.

### How it works

For each MISMATCH detected in Steps 1-6, the assigned **read-only reviewer** completes Step 7 in the same session. Analyze each mismatch independently, including when several items share a batch. The initial report includes the verdict, root cause, suggested direction, severity, confidence, evidence and affected keys; acceptance completes both alignment and divergence analysis.

#### Full and Delta Coverage Sets

> **Full mode note:** In full mode the coverage set contains `item:<unit>:<item>`, `unit:<unit>` (the unit-level scan), `code:<file>`, `design:<unit>:<file>` and `architecture:<unit>`. The tooling computes the run's reviewer units for the uncovered tasks — one per report kind and lens (one per reviewed file for verify quality) — and the coordinator joins the units of one lens and kind into a session before missioning them; a session's key set is split only when a session declares it exceeds that session's capacity (see `framework/verification_scope.md` §Coverage model → §Session capacity). A session that covers item(s) reports the alignment verdicts; each `MISMATCH` also names its affected keys and includes the completed Step 7 analysis.

> **Delta mode note:** A delta/repair run fixes the re-run coverage set (the stale keys plus the current keys the baseline never declared). Carried items bring their prior structured judgment into the run. The final synthesis consumes the new resolved judgments and the carried judgments together.

### Item reviewer protocol

For surplus mismatches, the first-principles analysis additionally evaluates:
- Is this a genuine design decision that belongs in the spec? → **spec_gap**
- Does this conflict with the spec's declared architecture or boundaries? → **needs_design**
- Does this duplicate or replace something the spec already describes differently? → **code_gap**

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
   - Truth ownership / shadow spec check: Does the mismatch involve fields, parameters, or internal structures of a collaborating unit? If so, does the collaborating unit export them in its promises (`## Promises` — the formal behavior carrier; acceptance items and appendices are not the contract)? If the spec is enumerating private/volatile internals of another unit without an exported contract anchor, the spec is a shadow specification (see `framework/spec_writing_guide.md` §14). File association check: does the declared implementation participate in this item's behavior? Shared files are permitted; retain the unique source for behavior and shared agreements.

4. Root cause analysis (choose the best fit):
   - Code is incomplete — spec intent is clear, code hasn't caught up
   - Spec is stale — code has evolved, spec wasn't updated
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

```
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
```

#### Mission Submission and Report Contract

Before reviewing uncovered keys in the `alignment` lens, the main agent generates `specflowctl gate-mission --run {run_id} --keys {keys} --format prompt` and sends its output verbatim to the assigned independent read-only reviewer. The mission supplies the session's steps (Steps 2, 3, 4, and 6 plus Step 7 for an item session; Steps 1 and 5 plus Step 7 for the unit-level scan), exact session input paths, the report contract and submission command. The reviewer performs alignment and any required divergence analysis before one `gate-submit`. Accepted keys are terminal: do not generate a second analysis mission for them. The final synthesis consumes their accepted reports; the coordinator does not assemble another analysis prompt or copy a mismatch table.

The item reviewer locates and analyzes the mismatch while executing its own steps (Steps 2, 3, 4, and 6) and completes Step 7 in the same session. It reads the mismatch point's enclosing function or structure, direct callers and callees, relevant tests, the item's spec section, sibling items, and shared definitions (error codes, types, enums, data models, rationale) within its session read refs. It reads whatever context it needs from its session read refs; reports carry no declarations. If a required file is not in `read_refs`, it returns `Verification could not complete — missing read ref: <repo-relative path>` without a verdict. The coordinator does not submit that report; it adds the path to the open run with `specflowctl gate-extend --run {run_id} --paths <path>` — every accepted session keeps its verdict, and the uncovered key re-missions with the extended read refs. The reviewer includes the root cause and repair direction in the initial item report. It must not modify files, run state-changing commands, or launch sub-agents.

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
  {item.id}: {file}: {declaration}   # declaration = `acceptance_item:{item.id}` for the item's own spec block, a section-region heading, 1-based closed line ranges, or "all"
  - These declarations travel verbatim into the verify cache: the item's own spec block is declared as `acceptance_item:{item.id}` (the item region — located by id, so editing or reordering other items never stales this judgment), section-region headings name regions of the unit's own main spec (recorded per item in the cache's `checks` mapping), line ranges and `all` declare code-file scope; `gate-submit` validates the declarations against that session's `read_refs`, and `gate-finalize` computes the CIDs and writes the cache. The declared regions must cover every region this judgment depended on, including called functions and referenced structures
Confidence: {high | medium | low}
Rationale: {2-3 sentence first-principles reasoning chain}
Signal layer: {condensed metadata summary, if collected}
```

The `Problem:` / `Evidence:` / `Impact:` / `Fix:` (or `Decision:`) fields are the finding block: the tool renders them as the finding's detail lines in the unified report skeleton (§Output Format), followed by the `root_cause:` / `direction:` / `confidence:` extra lines. A MISMATCH verdict carries one finding block per retained mismatch, in report order: an item session may retain several independent mismatches, and the unit-level scan one per Step 1 declaration-class or Step 5 surplus mismatch. `Problem:` answers the comparison step; `Evidence:` quotes each present side verbatim, and when one side is absent — a surplus construct has no spec declaration, a missing implementation has no code — its sub-line states the absence and the searched scope; exactly one of `Fix:` and `Decision:` is required and must match the suggested direction (spec_gap / code_gap → `Fix:`; needs_design / blocked → `Decision:` with at least one `Options:` entry).

**Failure path:** an item reviewer that cannot complete Steps 2, 3, 4, and 6 (or Step 7 for its mismatches) reports:
"Verification could not complete — {reason}" without a verdict. Do not submit
an incomplete report; resolve the missing input (extend the open run with
`specflowctl gate-extend`, or re-plan when the required surface itself
changed).

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

The independent final-synthesis executor may raise each retained finding's canonical severity conservatively per `framework/severity_policy.md` §9 before its synthesis result is accepted:
- Each finding's severity is assigned by the session that raised it; the final synthesis never lowers it
- On retain or merge it may raise a finding's canonical severity when the read evidence proves the impact is larger than the graded severity: read at least one target file beyond the surface it was graded on that its impact claim depends on (a caller in a dependency unit, the callee implementation, or the appendix governing the affected behavior), and verify the §9.3 boundary holds against the read evidence
- Evidence rules (§9.4): raising requires positive evidence read from the target; no evidence → keep the session-assigned severity
- Deterministic severity mappings (e.g. Step 1 declaration table rows) keep their contract-decided grade
- The canonical severities drive the derived counts and blocking; they are not restated as a report section

A failed relationship finding that restates a root cause already stated by an input finding must merge the input finding into it (`merged -> {new_id}`); the group is counted once. A failed relationship always reports item `FAIL` with a new retained finding, at any severity, while the `Cross-check:` summary is `FAIL` only when that new finding is P0/P1 — a failed item carrying only P2/P3 findings keeps the summary `<passed>/N PASS`.

#### Final Synthesis Mission and Tool Contract

When the plan assigns relationships or the primary pass produced findings, one independent read-only final session runs last (generated with `specflowctl gate-mission --run {run_id} --final --format prompt`, recorded with `specflowctl gate-submit --run {run_id} --session cross --keys cross --report PATH`; a run with neither assigned relationships nor findings finalizes directly). The final session checks only the assigned relationships and disposes existing findings; it does not repeat local checks. Its selected names and per-relationship dependency declarations are defined in `framework/verification_scope.md` §Final synthesis. Local PASS results do not remove this work. It consumes the primary sessions' structured findings and the complete source context:

1. Collect all accepted alignment/quality results and every carried judgment.
2. `gate-submit` verifies that the final synthesis disposes every input finding exactly once, supplies the relationship results and one reasoned `Quality conclusion` for every design and architecture key (including carried keys), and binds every consumed result digest. The tooling derives the effective status map from the terminal retained set — the report carries no status lines.
3. The main agent classifies the retained findings per §Batch classification (skip when the activation threshold is not met); this presentation classification cannot change severity, retention, or the gate result
4. Present the tool-derived consolidated findings summary (§Summary format)
5. Wait for the user's decision per HARD RULE 3a:
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
{body — Items / Scope / Coverage / divergence analysis}
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

When no candidate spec exists (verify against stable), using §Target-layer applicability:

```
1. Run Steps 1-6 against stable spec
2. If all items are ALIGNED and every declared code file passes → report PASS; `gate-finalize` writes the verify
   cache with `target: stable` (drift confirmation state consumed by
   `fresh@stable`; `mode: full`, severity counts at 0, `blocking: false`,
   whole-file hash + chunk evidence — same coverage sequence as Step 8)
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

After the run's sessions complete, determine cache action based on the highest severity MISMATCH (the finding-level verdict). The suggested direction (spec_gap / code_gap / needs_design / blocked) does not change the gate — blocking is determined by severity only. The cache `result` field uses the gate vocabulary: `pass` means no P0/P1 blocking findings; `fail` is the failure-record state, written by a delta re-run's FAIL and by a candidate full-run FAIL (`mode: full`, `basis: full` — see below and §Delta re-run).

==ATOM_BEGIN:cache_evidence_path_forms==
**Recording cache evidence — tool-generated:** gate cache entries are never agent-declared. `gate-finalize` records one entry per file of the run's input surface (the union of every coverage key's read refs and the cross-synthesis surface), each carrying the whole-file `hash` and the ordered content-defined chunk sequence of the current bytes, plus a per-check marker (check key + lens; failure status on failure records). Reports carry no `Dependency scope` lines; there is nothing for a reviewer to declare and nothing to validate against `read_refs`.

Spec objects that are resolved by name (a dependency unit's main spec or appendix, a rule file) are recorded as **logical references** instead of physical paths — `unit:{name}` for a unit main spec, `unit:{name}:appendix:{file}` for a unit appendix (the full appendix file base name without `.md`, e.g. `unit:auth:appendix:unit_auth_account_token_claims`), `rule:{id}` for a rule file. The run's own target files (the unit's own main spec and appendices; for a rule target, the candidate rule file and its stable sibling) and code files keep physical paths. A logical reference resolves at freshness time to the current-layer file (candidate first, stable fallback), so promoting the referenced unit or rule does not stale a cache whose recorded content is unchanged (see `framework/validation_cache.md` §Logical References).
==ATOM_END:cache_evidence_path_forms==

- **If all ALIGNED:** the coverage run writes the verify cache per `framework/validation_cache.md` format:
  - `gate-finalize` creates `docs/specs/meta/validation/unit/{name}/` as needed
  - `gate-finalize` assembles the cache from the accepted reports and the run input surface: one entry per input-surface file (whole-file hash + ordered chunk sequence) and one marker per executed judgment key, tagged with its lens (see §Step 8 above)
  - `gate-finalize` writes `verify_result.md` with `result: pass`, severity counts at 0, `target: candidate` (candidate round) or `target: stable` (stable-only mode), `mode: full`, `blocking: false`, whole-file hashes and chunk sequences, and the merged `alignment` + `quality` markers

- **If any P0/P1 MISMATCH exists (verify FAIL, full run, candidate target):** the finalize writes a failure record (`result: fail` + `blocking: true`, `mode: full`, `basis: full`; the per-item `status` map — `pass`/`fail` for every acceptance item and every declared code file; full runs re-execute local checks and may carry unchanged relationship judgments — and the file evidence are assembled by `gate-finalize` from the accepted reports) — the failure-recovery baseline. After the findings are resolved, `reverify@{unit}` re-checks the failed keys, persisted `invalidated_checks`, newly affected keys, current keys the baseline never declared, and explicit `--rerun` overrides, then carries the remaining `pass` keys over (see §Delta re-run and `framework/verification_scope.md` §Delta Runs → Failure recovery). This is safe because verify is anchored to the validated spec — carried keys' evidence is unchanged and not contradicted by a targeted P0/P1. Report blocking findings. Agent must stop and not proceed to promote. (A stable-only full FAIL writes the same failure record shape with the per-item status map — see §Stable-only mode.)

- **If all mismatches are P2/P3 (non-blocking, verify PASS):**
  - `gate-finalize` writes `verify_result.md` with `result: pass`, `blocking: false`, severity counts (`p0_count`...`p3_count`), `target: candidate` (candidate round) or `target: stable` (stable-only mode), `mode: full`, whole-file hashes and chunk sequences
  - Report findings as non-blocking — agent may continue (P2/P3 findings do not block promote).

- **Targeted runs (`:{keyword}`):** report findings only — do not publish a complete cache. PASS leaves cache state unchanged. On P0/P1, immediately run `specflowctl gate-invalidate --gate verify --unit {name} --target {candidate|stable} --check {item id}`: it deletes a pass cache or persists the contradicted item on a failure record and invalidates a matching open run. Blocking findings at any granularity mean promote must not proceed (`framework/validation_cache.md`).

### Delta re-run (reverify@{unit})

Candidate targets, plus stable-only targets with a usable baseline — a delta re-run against a stable-only target whose confirmation cache is MISSING reports the missing-baseline message — "No usable confirmation baseline. Run the full `{command}@{target}` (confirmation check) first, or `specflowctl fork {--unit|--rule} {name}` to start a new round." — and stops (see `framework/verification_scope.md` §Delta Runs → Layer applicability). Triggered by the user when the cache is STALE or BLOCKED (a failure record, or a salvaged pass cache whose `invalidated_checks` are pending). Follow §Delta Runs in `framework/verification_scope.md`:

1. **Preconditions** — the existing cache must have `mode: full` and one of the two baselines: `result: pass` (stale-cache recovery) or `result: fail` + `blocking: true` (failure-record recovery, §Failure recovery). A MISSING cache has no baseline — run `verify@{unit}` instead. A failure-record recovery of a full-run FAIL's record is anchored to the validated spec — the recovery's `pass`/`carried` items are trusted only because the unit's validate gate has passed (promote enforces this; see §Failure handling by gate role in `framework/validation_cache.md`).
2. **Plan (derived scope)** — run `specflowctl gate-plan --gate verify --unit {name} --target {candidate|stable} --mode delta` (or `--mode repair` for a failure record or a salvaged pass cache with pending `invalidated_checks`). The planner derives the re-run set from each conclusion's own judgment record: a conclusion re-runs when its pinned file, structural region, review protocol or invalidation state no longer holds, when the evidence its key currently requires is not inside the evidence the record read (a new file in the unit's declared surface), or when it has no baseline record; current keys the baseline never declared always run; explicit `--rerun` overrides are added; repair additionally forces the failed judgments, the persisted `invalidated_checks`, and the failed relationships. No review session is scheduled. It reports the derived re-run set, the carried keys, and any degradation or full-scope statement. A targeted P0/P1 is persisted immediately by `gate-invalidate`, so repair re-runs it without any remembered `--rerun` argument. A cache without the required chunk evidence is missing required recorded evidence and is treated as no baseline — the planner refuses the delta/repair and directs a full run. A failure record whose per-check status map is absent, invalid, or inconsistent with its judgment baseline covers the full local scope and re-runs the standing relationship conclusions. The derived set is the whole scope; no later step widens or narrows it.
3. **Execute** — execute the planned coverage keys (protocol: `framework/verification_scope.md` §Coverage model), generating each session's mission with `gate-mission`, submitting each report with `gate-submit --run <run_id> --session <session_id> --keys <keys> --report PATH`, and (when relationships are assigned or findings exist) generating the final synthesis with `gate-mission --run <run_id> --final`. Step 7 applies unchanged: a MISMATCH drives the divergence analysis, with the same input table and Context scope fill rule. Any P0/P1 MISMATCH → the finalize writes a **failure record** (`result: fail`, `blocking: true`, severity counts, findings body, `mode: full`, `basis: delta`, per-key `status` map — `fail`/`pass` for the re-run keys, `carried` for the carried-over ones, derived by `gate-finalize` from the accepted reports). Stop, present findings (same as full FAIL).
4. **On PASS** — `gate-finalize` rewrites `verify_result.md` with `result: pass`, `mode: full`, `basis: delta` (or `basis: repair`), `target: candidate` (stable-only target: `target: stable`), `blocking: false`, severity counts, a fresh `timestamp`, and a **complete** `files` list: fresh `hash` + `chunks` evidence for the re-run paths and the plan-time snapshot's evidence for the carried-over ones, merged by the tooling. A verify delta records no review record: the tooling derives its scope from each conclusion's own judgment-record pins. Logical references (`unit:{name}` / `unit:{name}:appendix:{file}` / `rule:{id}`) are carried over the same way. P2/P3 findings are carried by the severity counts exactly like a full run.

# Severity Policy

## 1. Purpose

This file defines the centralized severity levels used by Spec Flow findings and deviation reports.

It answers four questions:

1. what each severity level means
2. which review or verification flows use the same scale
3. how severity relates to blocking status
4. what a report must explain when it assigns a severity

This is a rule governance contract.
Executors must not invent a different severity meaning per command.

---

## 2. Scope

This policy applies when a Spec Flow review or verification output needs to grade a real problem.

By default it governs:

1. `spec_flow_review`
2. `spec_flow_design_review`
3. the `quality` lens of `verify` — uses the P0-P3 code review severity definitions below

It may also be reused by other governance flows if those flows explicitly say so.

It does not define:

1. `fallback_reason_code`
2. governance progression
3. whether a command may continue after binding validation

---

## 3. Core Principle

Severity answers only one question:

1. how harmful the confirmed problem is to flow correctness, behavior stability, or safe downstream work

Severity does not answer:

1. which fallback step is required
2. whether the issue came from truth drift, implementation drift, or evidence incompleteness
3. whether a user should prefer one product choice over another

Blocking status must still be stated explicitly.
Do not assume that severity alone fully determines the next action.

---

## 4. Severity Levels

### 4.1 `P0`

Use `P0` for:

1. main-chain break
2. truth conflict
3. key gate distortion
4. governance ambiguity that can make executors run the wrong flow or skip a required gate

Plain meaning:

1. the flow is not safely controllable until this is repaired

### 4.2 `P1`

Use `P1` for:

1. behavior or implementation meaning that is unstable enough to block safe downstream planning, implementation, verification, or promotion
2. verification deviations that already threaten the current round's externally meaningful result

Plain meaning:

1. the flow structure still exists
2. but the current round must not continue past the affected gate

### 4.3 `P2`

Use `P2` for:

1. issues that do not block the current next gate by themselves
2. but materially harm review stability, readability, maintainability, or future closure

Plain meaning:

1. downstream work may still continue if no higher-severity blocker exists
2. but the repository is accumulating governance or verification debt

### 4.4 `P3`

Use `P3` for:

1. minor elaboration or clarity issues
2. low-impact reporting gaps

Plain meaning:

1. the issue is real
2. but it does not materially change current flow control or review safety

---

## 5. Blocking Relationship

Severity and blocking are related but not identical.

Rules:

1. `P0` is normally blocking.
2. `P1` is normally blocking for the affected downstream step.
3. `P2` is normally non-blocking unless a command-specific rule says otherwise.
4. `P3` is normally non-blocking.
5. reports must still state blocking status explicitly instead of making the reader infer it.

---

## 6. Required Explanation Fields

When a governed flow assigns a severity to a real problem, the report should explain:

1. background
2. what happened
3. impact
4. recommended fix
5. why that fix is the minimal correct fix
6. whether the issue is blocking

Commands or flows may add more required fields, but must not weaken this baseline.

Gate findings (`validate` / `verify`) carry this baseline through the unified finding block (atom source `framework/_atoms/misc/report_skeleton.md`, delivered into each command checklist): `problem:` states what happened, `evidence:` carries the background and the proof, `impact:` states the impact, `fix:` or `decision:` states the recommended fix or the decision the user must make, and the report header states blocking explicitly. Minimality is governed by the flow's own fix execution rules (e.g. `framework/unit_verify_checklist.md` §Fix execution rules).

---

## 7. Relationship To Other Files

This policy works together with:

1. `framework/spec_flow_review.md`
2. `framework/spec_flow_design_review.md`

Priority rules:

1. the active review or command file decides whether grading is required
2. this file defines the shared meaning of `P0 / P1 / P2 / P3`
3. command-local or flow-local text may add required report fields, but must not redefine the shared severity meaning

---

## 8. Non-Goals

This file does not:

1. define behavior truth
2. define verification evidence formats
3. replace `fallback_reason_code`

---

## 9. Severity Consistency Check

### 9.1 Purpose

Severity is assigned by the session that raises a finding. This check verifies the assigned grade's implied impact claim against the global context before it takes effect (before cache write or final output), so a grading that only looks right inside the local review surface cannot silently pass. When the final synthesis retains or merges findings, it may raise a retained finding's canonical severity conservatively — never lower it; the raised value is canonical.

This check answers one question per finding:

1. does the severity's implied impact claim hold against the full context the flow operates on?

It is distinct from existence validation (cross-check). Existence validation decides whether the finding is real; this check decides whether the grading of a real finding is accurate.

### 9.2 Scope

This check applies to every flow whose severity grading is part of a finding contract, where the grading takes effect on a gate outcome, cache content, or the flow's final output. Each such flow defines where the check executes inside its own procedure file and references this section; that definition is the only adoption mechanism. Report-priority grading outside a finding contract (e.g. the `spec_flow_issues` triage severity) is not in scope.

Flows that currently define an execution position:

1. `spec_flow_review` — full-scope procedure step 10 (`framework/spec_flow_review.md`)
2. `spec_flow_design_review` — procedure step 13 (`framework/spec_flow_design_review.md`)
3. the `quality` lens of `verify` — the final synthesis (`framework/unit_verify_checklist.md` §Final synthesis, `framework/verification_scope.md` §Final synthesis)
4. the `alignment` lens of `verify` — Step 7 analysis / final synthesis (`framework/unit_verify_checklist.md`)
5. `validate` — P0 adjudications (P1 is the contract-decided default and is not re-graded) and advisory findings (Check 1 hygiene) (`framework/unit_validate_checklist.md`)
6. `validate` (rule) — P0 adjudications (P1 is the contract-decided default and is not re-graded) (`framework/rule_validate_checklist.md`)
7. scoped review — conclusion stage (`framework/governance/review_scope.md`)

The list is a record of current wiring, not the coverage definition. Coverage is decided by the first paragraph: a flow is in scope when its severity grading is part of a finding contract, and its execution position is wherever its own procedure file places this check. This section defines the shared meaning, boundaries, and evidence rules.

Deterministic severity mappings (e.g. the `verify` Step 1 declaration table) are contract-decided and are not re-graded by this check. Judgment-based severities in flows covered by the first paragraph (subagent or reviewer grading) are always in scope.

### 9.3 Severity Boundaries

Each severity implies an impact claim. The check verifies the claim against read evidence:

| Severity | Implied impact claim | What to verify from the global context | The grade is too low when |
|---|---|---|---|
| P0 | The impact is determinable from code structure alone; no runtime data or inference is needed | The impact does not depend on runtime conditions; no protective path in the read target invalidates it | — (P0 is the ceiling) |
| P1 | The impact inevitably surfaces over time and threatens downstream work beyond the reviewed surface | The impact reaches real consumers or dependent units; the affected gate outcome is at risk | Impact reaches outside the reviewed surface and breaks an externally meaningful result → raise to P0 |
| P2 | The impact is real but does not touch correctness | The issue truly does not affect correctness | Issue affects correctness → raise to P1 |
| P3 | The issue does not materially harm maintainability | The issue truly does not affect maintainability | Issue materially harms maintainability → raise to P2 |

### 9.4 Evidence Rules

1. **Raising requires positive evidence** — the checker must read concrete code or document content proving the impact is larger than the graded severity. "Possible" or "might" reasoning never raises.
2. **No evidence → keep the severity assigned by the raising session.** The check does not re-guess the grade.

### 9.5 Execution Rules

1. **Execution position.** In a coverage gate run (`validate`, `verify`) the check is the independent final synthesis, which runs for assigned relationships or findings. Severity verification applies to retained, merged, or newly discovered relationship findings; a run with neither relationships nor findings skips the final session. Rule validate has no final synthesis: its single checks session assigns each finding's severity directly and no later step re-grades it. In `spec_flow_review`, `spec_flow_design_review`, and scoped review it is the reviewer or main agent that holds the flow's global context, as those flows' procedure files define. Advisory findings that never enter the final synthesis — validate's Check 1 hygiene WARNING — are graded by the session executor that produced the check line (see `framework/unit_validate_checklist.md` §Present Findings).
2. For each finding, the checker must read at least one target file beyond the surface the finding was graded on (caller, callee, consumer, dependent unit, or governing document). For document-judged findings (e.g. validate advisory findings), the beyond-surface read is the section or appendix the finding's impact claim depends on. Re-reasoning from already-read context does not count as a check.
3. The check runs after existence validation (the final synthesis, when it runs) and before the cache write or final output, so a raised severity determines blocking status and cache content.
4. Severity is a semantic judgment; tooling does not participate (see `tooling_execution_policy.md`).

---

## 10. Code Review Severity Extension (Quality Lens)

The `quality` lens of `verify` uses the same P0-P3 scale with code-review-specific definitions.

==ATOM_BEGIN:spec_review_standard==
## Quality Lens Standard

The `quality` lens separates reusable public code facts from unit-specific design judgments and the unit's overall architecture assessment. The complete protocol is `framework/shared_judgments.md`.

### 1. Core Principle

`code:<file>` records facts and potential problems without a unit-private rationale. `design:<unit>:<file>` treats that unit's spec as rationale: actively check its requirements and retain or exclude every public observation from its assigned file's immutable record with evidence. Public execution batches do not enlarge the design session's inputs. A unit-specific exclusion never removes a public observation. A co-batched session holds a file's `code:<file>` key and its unit's `design:<unit>:<file>` key together: the same reviewer collects the facts and then judges the unit design against them in one pass — the facts stay rationale-free and publish as the file's public record, so other units can still reuse them. `architecture:<unit>` assesses Dimension 8 once for the entire unit. The alignment lens treats the spec as authority.

### 2. Pre-review Setup

Public checks read the complete file evidence surface fixed by the mission, including related callers, callees, dependencies, tests and applicable public rules. Do not read unfinished peer designs. A missing evidence path is reported (`Verification could not complete — missing read ref: <repo-relative path>`), not judged around; the coordinator adds it to the open run with `specflowctl gate-extend`.

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

Public and design sessions contain exactly one `File: {assigned check key}` block per assigned file. Public blocks contain `conclusion: FACTS`, `facts` and potential observations — no dependency-scope lines: the whole public evidence surface is recorded as their dependency by the tooling. Design blocks contain a quality conclusion, `spec_requirements`, `gate_findings`, an evidence-backed disposition for each public observation, new findings and dependency scopes. Architecture uses one `Unit: architecture:{unit}` block, the six Dimension 8 assessments, conclusion, `gate_findings`, `Suppressed by spec (N)` and dependency scopes. Batching never shares one subject's verdict or findings with another.

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

**Dependency scope report:** In addition to findings, every design or architecture sub-agent reports the read scope of its session — for the reviewed file, the section-region headings (or 1-based closed line ranges; `all` when the assessment covered the whole file) its review judgment actually depended on:

```
Dependency scope:
  {check key}: {file}: {declaration}   # check key = design:<unit>:<file> or architecture:<unit>
```

Every code input uses an exact whole-file fingerprint, including evidence read by a design, architecture or acceptance judgment. Public code checks (`code:<file>`) carry no scope lines: each check's own public evidence surface — the read refs of its coverage key — is its dependency, whole-file, and `gate-submit` records it directly from that coverage key's read refs. Design and architecture sessions declare the scope they read; spec dependencies may use chapters or acceptance-item regions. `gate-submit` validates declared scopes against that session's `read_refs` (not merely the run-wide snapshot), and `gate-finalize` computes the CIDs and records the per-check breakdown (check key = the assigned task key, lens = `quality`) in the cache's `checks` mapping — section headings become section-region dependencies for the unit's own main spec, line ranges become chunk declarations (see `framework/validation_cache.md` §Format → Per-check evidence); the declared ranges must cover every region the review judgment depended on, including called functions and referenced structures.
==ATOM_END:spec_review_standard==

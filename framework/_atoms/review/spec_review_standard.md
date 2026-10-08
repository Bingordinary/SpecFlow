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

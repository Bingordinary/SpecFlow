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
{severity} {location} — {issue}
      spec_context: {relevant design context from spec, if any}
      recommendation: {fix suggestion}
```

Each finding contains:
- `severity`: P0-P3
- `location`: file path + line number
- `issue`: description of the problem
- `spec_context`: (optional) relevant design context from the spec, helps the user understand the code-design relationship
- `recommendation`: fix suggestion
- `fact_anchor`: (required for P3) the reproducible repository fact, comparison or governing reference, violating location, and relationship that proves the P3 discrepancy

**Dependency scope report:** In addition to findings, every sub-agent reports the read scope of its slice — per review dimension, for each file it read, the section-region headings (or 1-based closed line ranges; `all` when the assessment covered the whole file) its review judgment actually depended on:

```
Dependency scope:
  {dimension}: {file}: {declaration}   # declaration = section heading, line ranges, or "all"
```

Review judgments commonly cover whole files (a code quality assessment has no partial scope) — report `all` honestly in that case; the cache's `deps` then covers the whole file by design. The main agent carries this report over and uses it when writing the review cache — section headings become `--section` declarations for the unit's own main spec (recorded per dimension in the cache's `checks` mapping), line ranges become `--ranges` (see `framework/spec_review_checklist.md` §8); the declared ranges must cover every region the review judgment depended on, including called functions and referenced structures.

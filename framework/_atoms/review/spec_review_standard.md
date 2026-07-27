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

## 5. Severity Levels

| Level | Definition | Characteristic | Example | Promote Gate |
|-------|-----------|----------------|---------|-------------|
| **P0** | Definitively causes production misbehavior | Determined from code structure alone, no runtime data needed | Null pointer dereference, deadlock, resource leak, race condition, incorrect lock usage, use-after-close | 🚫 Block |
| **P1** | Inevitably causes maintenance pain or high-probability bugs | Latent but will surface over time | Silently swallowed exceptions, broken error propagation paths, large-scale logic duplication | 🚫 Block |
| **P2** | Real but not severe | Affects readability and maintainability, not correctness | Mysterious Name, Feature Envy, localized Primitive Obsession, small Data Clumps | ✅ Don't block |
| **P3** | Style or clarity | Does not affect correctness, does not significantly harm maintainability | Unused import, minor naming inconsistency, stale comment | ✅ Don't block |

P0/P1 findings block regardless of scoped or full mode. The only additional gate for promote is that the cache must come from a `:full` run.

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

# Spec Review Checklist

## Overview

When an agent executes `spec_review {unit}`, it uses the spec-aware code quality review defined in this file. This file is referenced by `framework/concepts.md` — the agent reads this file at review time, not proactively.

## Mode Selection

| Trigger | Mode | What to execute |
|---------|------|-----------------|
| `spec_review {unit}` | scoped (default) | git diff HEAD → match changed files to `affects.files` and `implementation_surface` → review those files using the standard defined below |
| `spec_review {unit}:full` | full | Read all files referenced in the candidate spec's `affects.files` and `implementation_surface` across all acceptance items → review those files using the standard defined below |

**Scoped selection logic:** Run `git diff HEAD`. Identify files matching `affects.files` or `implementation_surface` in the candidate spec. Review identified files using the standard below. No match → report "changed files not referenced in spec" and suggest full run.

**Edge cases:** No git changes, scoped cache fresh → report still valid. No git changes, no cache → auto fallback to full.

**Output:** Prefix with `Mode: scoped` or `Mode: full`. For scoped: append note "This is not a full review. Run `spec_review {unit}:full` for complete review."

**Cache:** see `framework/validation_cache.md` for format.

## Execution Rules

- **Subagent permissions:** may inspect file content, search text by pattern, locate files by name pattern, and query git history. Must NOT modify files or execute commands that change state.
- **Cross-check:** See §7.
- **Failure Behavior:** If subagent encounters an error (cannot read target files, target unit not found, review checklist missing), report "Review could not complete — <reason>". Do not write review cache. Advise resolving the issue before retrying. This is distinct from review findings — when the review runs and finds P0/P1 issues, the subagent completed normally (the output is PASS or FAIL per the gate rules below), not a subagent failure.
- Each finding reports P0-P3 severity with code references.
- Suppressed findings are listed separately under "Suppressed by spec".

## Output Format

```
Mode: scoped | full
Review result: PASS | FAIL
Findings: N (P0: 0 | P1: 0 | P2: 0 | P3: 0)

Findings:
  [{severity}] {location} — {issue}
        spec_context: {design context, if any}
        recommendation: {fix suggestion}

Suppressed by spec:
  {location} — {issue} → {suppression_reason}, {spec_ref}

Summary:
  Total potential findings: N
  Suppressed by spec: N
  P0: N | P1: N | P2: N | P3: N

Gate: review {blocks|does not block} promote ({reason})
```

**PASS:** No P0 or P1 findings exist.
**FAIL:** One or more P0 or P1 findings exist — blocks promote.

## Gate Rules

| Condition | Result |
|-----------|--------|
| P0 or P1 findings exist | FAIL — blocks promote regardless of scoped/full |
| Only P2 or P3 findings | PASS — does not block promote |
| No findings | PASS — does not block promote |

For promote gate: only `:full` mode cache with PASS result satisfies the promote requirement. A scoped PASS cache is informational only.

==ATOM_BEGIN:spec_review_standard==
# Spec Review Standard

## 1. Core Principle

`spec_review` audits code quality. Its single difference from ordinary code review: for every potential finding, it checks the spec for a design rationale. If the spec explains why the code is written that way, the finding is suppressed.

It does NOT do:
- `spec_validate` work (checking spec quality)
- `spec_verify` work (checking spec-code alignment)

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
==ATOM_END:spec_review_standard==

## 7. Cross-Check

### 7.1 Purpose

After collection, before output — validate that each finding holds in the full codebase and full document set, not just in the local review context.

Both scoped and full mode need this:
- **Scoped** — reviews only git-diff files; a changed file may call code outside the diff set. The agent cannot assume the callee lacks protections without reading it.
- **Full** — distributes work to sub-agents; each sees only its slice, not cross-unit implementation details.

### 7.2 Process

1. **跨文件真实性验证** — For each finding whose issue asserts "X lacks Y capability" or "parameter Z may not be handled", read the callee/target implementation. If the target handles the concern (nil guard, empty-string path, idempotent close, etc.), the finding is a false positive.

2. **跨文档一致性验证** — For each finding that relies exclusively on the spec main body, check appendices and related documents for supplementary or overriding references. If the full document set resolves the issue, the finding is invalid.

3. **移除误报** — Findings that dissolve under cross-check are removed from output. Not demoted to notes — a false positive has no place in the review result.

### 7.3 Complexity

Lightweight: the main agent reads 1-2 target files per finding. No sub-agent re-launch or full re-review.

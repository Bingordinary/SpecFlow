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
3. `review` — uses the P0-P3 code review severity definitions below

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

## 9. Code Review Severity Extension

`review` uses the same P0-P3 scale with code-review-specific definitions.

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

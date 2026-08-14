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

## 9. Severity Consistency Check

### 9.1 Purpose

Severity is assigned once during finding collection. This check re-validates each assigned severity from the global context before it takes effect (before cache write or final output), so a grading that only looks right inside the local review surface cannot silently pass.

This check answers one question per finding:

1. does the severity's implied impact claim hold against the full context the flow operates on?

It is distinct from existence validation (cross-check). Existence validation decides whether the finding is real; this check decides whether the grading of a real finding is accurate.

### 9.2 Scope

This check applies to every flow whose severity grading is part of a finding contract, where the grading takes effect on a gate outcome, cache content, or the flow's final output. Each such flow defines where the check executes inside its own procedure file and references this section; that definition is the only adoption mechanism. Report-priority grading outside a finding contract (e.g. the `spec_flow_issues` triage severity) is not in scope.

Flows that currently define an execution position:

1. `spec_flow_review` — full-scope procedure step 10 (`framework/spec_flow_review.md`)
2. `spec_flow_design_review` — procedure step 13 (`framework/spec_flow_design_review.md`)
3. `review` — cross-check §7.4 (`framework/spec_review_checklist.md`)
4. `verify` — Step 7 analysis collection (`framework/unit_verify_checklist.md`)
5. `validate` — P0 adjudications (P1 is the contract-decided default and is not re-graded) and Check 2 Step 4 advisory findings (`framework/unit_validate_checklist.md`)
6. `validate` (rule) — P0 adjudications (P1 is the contract-decided default and is not re-graded) (`framework/rule_validate_checklist.md`)
7. scoped review — conclusion stage (`framework/governance/review_scope.md`)

The list is a record of current wiring, not the coverage definition. Coverage is decided by the first paragraph: a flow is in scope when its severity grading is part of a finding contract, and its execution position is wherever its own procedure file places this check. This section defines the shared meaning, boundaries, evidence rules, and record contract.

Deterministic severity mappings (e.g. the `verify` Step 1 declaration table) are contract-decided and are not re-graded by this check. Judgment-based severities in flows covered by the first paragraph (subagent or reviewer grading) are always in scope.

### 9.3 Severity Boundaries

Each severity implies an impact claim. The check verifies the claim against read evidence:

| Severity | Implied impact claim | What to verify from the global context | Claim fails when |
|---|---|---|---|
| P0 | The impact is determinable from code structure alone; no runtime data or inference is needed | The impact does not depend on runtime conditions; no protective path in the read target invalidates it | Impact requires runtime data or inference → downgrade to P1 |
| P1 | The impact inevitably surfaces over time and threatens downstream work beyond the reviewed surface | The impact reaches real consumers or dependent units; the affected gate outcome is at risk | Impact reaches outside the reviewed surface and breaks an externally meaningful result → upgrade to P0; impact is local with no downstream consumer → downgrade to P2 |
| P2 | The impact is real but does not touch correctness | The issue truly does not affect correctness | Issue affects correctness → upgrade to P1; issue is style-level only → downgrade to P3 |
| P3 | The issue does not materially harm maintainability | The issue truly does not affect maintainability | Issue materially harms maintainability → upgrade to P2 |

### 9.4 Evidence Rules

1. **Upgrade requires positive evidence** — the checker must read concrete code or document content proving the impact is larger than graded. "Possible" or "might" reasoning never upgrades.
2. **Downgrade requires completed reading** — the checker must finish reading the target file and confirm the protective path exists, the consumer is absent, or the impact is contained. "Not found" without reading never downgrades.
3. **No evidence → keep the original severity.** The check confirms grading; it does not re-guess it.
4. **One level per adjustment, at most two iterations.** After an adjustment, run the boundary check for the new severity once. The second result is final. A check must not keep moving a finding across levels.

### 9.5 Execution Rules

1. The checker is the main agent (or reviewer) that already holds the flow's global context; no new subagent is launched for this check.
2. For each finding, the checker must read at least one target file beyond the surface the finding was graded on (caller, callee, consumer, dependent unit, or governing document). For document-judged findings (e.g. validate advisory findings), the beyond-surface read is the section or appendix the finding's impact claim depends on. Re-reasoning from already-read context does not count as a check.
3. The check runs after existence validation (cross-check) and before the cache write or final output, so adjusted severities determine blocking status and cache content.
4. Severity is a semantic judgment; tooling does not participate (see `tooling_execution_policy.md`).

### 9.6 Record Contract

Every finding records one of:

1. `confirmed` — severity stays after the boundary check
2. `adjusted: {Px} → {Py}` — with the evidence file read and a one-line reason

The record must appear in the flow's output (or cache findings body) so the review can trace that the check actually ran. A finding with no record is treated as unconfirmed, unless the flow's procedure file explicitly exempts its severity level from this check; an exempted finding is reported as graded with no confirmation record, and may not claim a blocking status without first being re-graded through the confirmation path.

---

## 10. Code Review Severity Extension

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

### Dimension 8: Architectural Design Quality

Whether the implemented code structure forms an acceptable architecture for the unit's declared responsibility. This dimension evaluates the design surface of the code — module boundaries, responsibility organization, abstraction levels, dependency clarity, and extension landing points — as an overall architecture assessment, not as a smell checklist.

- **Assessment object**: The code under `implementation_surface` and `affects.files` — package structure, module boundaries, how responsibilities are organized, how components depend on each other, and how the structure matches the repository's established engineering patterns (layering, naming, error-handling conventions)
- **P0/P1 findings (gate-level, judged from code structure and spec declarations alone)**:
  - Spec-recorded architectural intent (e.g., declared layering, module boundaries) is implemented in a structure that has drifted beyond recognition → P0/P1
  - The unit reaches past the `unit_refs` boundary and reaches into a dependency unit's internal implementation → P1
  - Internal implementation organization is severely disconnected from the unit's declared responsibility (e.g., a "config loading" unit's surface contains business logic) → P1
- **P2/P3 findings (advisory, taste-level)**: boundaries cut less naturally than the behavior domains suggest, abstraction levels slightly off, extension landing points less explicit than they could be
- **Spec interaction**: Spec-recorded architectural decisions with a conforming implementation are NOT re-questioned here — the recorded decision is authoritative (validated by `validate`). Assessment focuses on implementation drift from recorded intent and on code structure the spec does not cover
- **Not considered**: Design quality of spec-recorded decisions themselves (owned by `validate` Check 2); acceptance alignment (owned by `verify`)

## 5. Severity Levels

| Level | Definition | Characteristic | Example | Promote Gate |
|-------|-----------|----------------|---------|-------------|
| **P0** | Definitively causes production misbehavior | Determined from code structure alone, no runtime data needed | Null pointer dereference, deadlock, resource leak, race condition, incorrect lock usage, use-after-close | Block |
| **P1** | Inevitably causes maintenance pain or high-probability bugs | Latent but will surface over time | Silently swallowed exceptions, broken error propagation paths, large-scale logic duplication | Block |
| **P2** | Real but not severe | Affects readability and maintainability, not correctness | Mysterious Name, Feature Envy, localized Primitive Obsession, small Data Clumps | Don't block |
| **P3** | Style or clarity | Does not affect correctness, does not significantly harm maintainability | Unused import, minor naming inconsistency, stale comment | Don't block |

P0/P1 findings block promote. The promote gate additionally requires a cache written by a full run — targeted keyword re-reviews do not write a cache, so they never satisfy the gate.

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

**Dependency scope report:** In addition to findings, every sub-agent reports the read scope of its slice — per review dimension, for each file it read, the section-region headings (or 1-based closed line ranges; `all` when the assessment covered the whole file) its review judgment actually depended on:

```
Dependency scope:
  {dimension}: {file}: {declaration}   # declaration = section heading, line ranges, or "all"
```

Review judgments commonly cover whole files (a code quality assessment has no partial scope) — report `all` honestly in that case; the cache's `deps` then covers the whole file by design. The main agent carries this report over and uses it when writing the review cache — section headings become `--section` declarations for the unit's own main spec (recorded per dimension in the cache's `checks` mapping), line ranges become `--ranges` (see `framework/spec_review_checklist.md` §8); the declared ranges must cover every region the review judgment depended on, including called functions and referenced structures.
==ATOM_END:spec_review_standard==

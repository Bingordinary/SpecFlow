# Spec Review Checklist

## Overview

When an agent executes `review@{unit}`, it uses the spec-aware code quality review defined in this file. This file is referenced by `framework/concepts.md` — the agent reads this file at review time, not proactively.

## Mode Selection

| Trigger | Mode | What to execute |
|---------|------|-----------------|
| `review@{unit}` | scoped (default) | git diff HEAD → match changed files to `affects.files` and `implementation_surface` → review those files using the standard defined below |
| `review@{unit}:full` | full | Read all files referenced in the candidate spec's `affects.files` and `implementation_surface` across all acceptance items → review those files using the standard defined below |

**Scoped selection logic:** Run `git diff HEAD`. Identify files matching `affects.files` or `implementation_surface` in the candidate spec. Review identified files using the standard below. No match → report "changed files not referenced in spec" and suggest full run.

**Edge cases:** No git changes, scoped cache fresh → report still valid. No git changes, no cache → auto fallback to full.

**Output:** Prefix with `Mode: scoped` or `Mode: full`. For scoped: append note "This is not a full review. Run `review@{unit}:full` for complete review."

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

Architecture assessment:
  conclusion: acceptable | needs_attention | unacceptable
  module_boundaries: {assessment} — {basis}
  responsibility_organization: {assessment} — {basis}
  dependency_clarity: {assessment} — {basis}
  abstraction_level: {assessment} — {basis}
  extension_landing_points: {assessment} — {basis}
  engineering_patterns: {assessment} — {basis}
  gate_findings: {P0/P1 findings from Dimension 8, if any}

Findings:
  Batch group (N items) — fix does not change runtime behavior, suggested for batch handling:
    - {location}: {one-line fix} (P3, based on: {evidence location})
    ...
  Decision group (M items) — decided one by one:
    [{severity}] {location} — {issue}
          spec_context: {design context, if any}
          recommendation: {fix suggestion}
    ...

Suppressed by spec:
  {location} — {issue} → {suppression_reason}, {spec_ref}

Summary:
  Total potential findings: N
  Suppressed by spec: N
  P0: N | P1: N | P2: N | P3: N

Severity check (§7.4):
  confirmed: N | adjusted: N
  - {location} — adjusted {Px} → {Py}, evidence: {file}, reason: {one line}

Gate: review {blocks|does not block} promote ({reason})
```

**Architecture assessment (Dimension 8):** Always produced in full mode; in scoped mode produced when any in-scope file participates in a Dimension 8 gate finding. The assessment reports the code structure of the reviewed surface per Dimension 8 in §4 below. Gate-level architectural defects (P0/P1) appear both in the assessment's `gate_findings` and in the Findings section; taste-level observations appear as P2/P3 findings only. A spec-recorded architectural decision with a conforming implementation is not re-questioned here.

**Conclusion mapping:** any Dimension 8 P0/P1 gate finding → `unacceptable`; P2/P3 only → `needs_attention`; none → `acceptable`. The conclusion does not change the Gate Rules table — the gate remains decided by P0/P1 findings.

When batch grouping is inactive (threshold not met), the Findings section uses the flat format:

```
Findings:
  [{severity}] {location} — {issue}
        spec_context: {design context, if any}
        recommendation: {fix suggestion}
  ...
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

1. **Cross-file authenticity check** — For each finding whose issue asserts "X lacks Y capability" or "parameter Z may not be handled", read the callee/target implementation. If the target handles the concern (nil guard, empty-string path, idempotent close, etc.), the finding is a false positive.

2. **Cross-document consistency check** — For each finding that relies exclusively on the spec main body, check appendices and related documents for supplementary or overriding references. If the full document set resolves the issue, the finding is invalid.

3. **Remove false positives** — Findings that dissolve under cross-check are removed from output. Not demoted to notes — a false positive has no place in the review result.

### 7.3 Complexity

Lightweight: the main agent reads 1-2 target files per finding. No sub-agent re-launch or full re-review.

### 7.4 Severity Consistency Check

After the cross-check removes false positives, the main agent confirms each retained finding's severity before classification and cache write, per `framework/severity_policy.md` §9.

For every retained finding (P0-P3):

1. Read at least one target file beyond the reviewed surface that the finding's impact claim depends on (caller, callee, consumer, or the spec document governing the affected behavior)
2. Verify the severity boundary from `severity_policy.md` §9.3 holds against the read evidence
3. Record the result per finding:
   - `confirmed` — severity stays
   - `adjusted: {Px} → {Py}` — with the evidence file read and a one-line reason
4. Recompute the gate result (P0/P1 presence) from the adjusted severities before §8 writes the cache

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

## 8. Write review cache (main agent)

After cross-check (§7) and batch classification (§Batch classification) complete:

1. Compute gate result:
   - P0 or P1 findings exist → `result: fail`, `blocking: true`
   - Otherwise → `result: pass`, `blocking: false`

2. Write `docs/specs/meta/validation/unit/{name}/review_result.md` per `framework/validation_cache.md` format:
   - Create `docs/specs/meta/validation/unit/{name}/` directory if needed
   - Include `mode: scoped|full`, severity counts, `blocking`, file hashes
   - Include full findings body (cannot be omitted — required for promote gate detail)

Review always writes cache regardless of pass/fail.

**Cache record contract:** The cache records the file state read during the review run — it is independent of the user's decision outcome:

- Collect file hashes from the files read during the review run (same collection point as verify Step 8)
- Write the cache before applying any user-approved fixes — presenting findings and waiting for the user's decision is presentation only and does not gate the cache write
- Any fix applied after the cache write makes the cache stale (promote's hash check fails) — a full review re-run is required before promote. This is a promote-gate requirement enforced when the user triggers promote; it does not authorize automatic re-review after fixes. The agent must not re-run review on its own initiative (see HARD RULE 2 in `framework/concepts.md`). After a fix, the agent may propose a scoped re-review (`review@{unit}`) and waits for the user to trigger it; the `:full` re-run is triggered by the user when deciding to promote.

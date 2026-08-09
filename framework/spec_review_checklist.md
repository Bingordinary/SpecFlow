# Spec Review Checklist

## Overview

When an agent executes `review@{unit}`, it uses the spec-aware code quality review defined in this file. This file is referenced by `framework/concepts.md` — the agent reads this file at review time, not proactively.

## Mode Selection

| Trigger | Mode | What to execute |
|---------|------|-----------------|
| `review@{unit}` | full | Read all files referenced in the candidate spec's `affects.files` and `implementation_surface` across all acceptance items → review those files using the standard defined below |
| `review@{unit}:{keyword}` | targeted | Match keyword to a file name in `affects.files` or `implementation_surface` → review that file using the standard defined below. Does not write a cache. |

**Keyword domain:** review keywords resolve to code file names from the candidate spec's `affects.files` or `implementation_surface` (e.g., `:login.go` → review `src/auth/login.go`). A feature-name keyword is mapped to the files behind that feature. A keyword matching no file is a no-match — ask the user for clarification.

**Output:** Targeted runs report only the requested file(s) and note "This was a targeted check — no cache was written. Run `review@{unit}` for a complete review."

**Cache:** see `framework/validation_cache.md` for format.

## Execution Rules

- **Subagent permissions:** may inspect file content, search text by pattern, locate files by name pattern, and query git history. Must NOT modify files or execute commands that change state.
- **Cross-check:** See §7.
- **Failure Behavior:** If subagent encounters an error (cannot read target files, target unit not found, review checklist missing), report "Review could not complete — <reason>". Do not write review cache. Advise resolving the issue before retrying. This is distinct from review findings — when the review runs and finds P0/P1 issues, the subagent completed normally (the output is PASS or FAIL per the gate rules below), not a subagent failure.
- Each finding reports P0-P3 severity with code references.
- Suppressed findings are listed separately under "Suppressed by spec".

## Output Format

==ATOM_BEGIN:report_skeleton==
## Unified Report Skeleton

All quality-gate reports (validate, verify, review) share the same report skeleton below. The header, the `Blocking promote` line, and the `Next step` line are identical across commands; the verdict vocabulary, the key counts, and the body content are command-specific and defined in each checklist file.

```
────────────────────────────────────────────
{command}@{target} · {mode} · {layer}
Result: {verdict}
Blocking promote: yes | no
Key counts: {command-specific counts}
────────────────────────────────────────────
{body}
────────────────────────────────────────────
Dependency scope:
  {file}: {lines}   # 1-based closed line ranges the judgment depended on; "all" for whole-file judgments
────────────────────────────────────────────
Findings:
  {batch group / decision group per this file's batch classification, or flat when grouping is inactive}
Summary: {command-specific summary counts}
────────────────────────────────────────────
Next step: {concrete next command with reason, or "None"}
────────────────────────────────────────────
```

**Field definitions:**

- `{command}@{target}` — the command and target that produced this report, e.g. `validate@user_auth`, `verify@user_auth`, `review@user_auth`. Commands: `validate`, `verify`, `review`. Targets: unit or rule name.
- `{mode}` — `full` for full runs; `targeted (user requested: {keyword})` for targeted runs.
- `{layer}` — the spec layer checked: `candidate` | `stable`.
- `{verdict}` — each command keeps its own verdict vocabulary on the unified line: validate — `PASS | FAIL` (unit targets add the `(fix_required | blocked)` resolution defined in `framework/unit_validate_checklist.md`); verify — `PASS | FAIL`; review — `PASS | FAIL`.
- `Blocking promote: yes | no` — `yes` when the result blocks promote (validate FAIL; verify P0/P1 findings; review P0/P1 findings); `no` otherwise.
- `{command-specific counts}` — validate: `Failed checks: N / Total findings: M / Advisory findings: K`; verify: `Blocking mismatches: N / Non-blocking mismatches: N`; review: `Findings: N (P0: a | P1: b | P2: c | P3: d)`.
- `{body}` — command-specific content defined in this file's body format section (validate: one line per check; verify: Items / Scope / Integrity / Coverage / first-principles divergence analysis; review: Architecture assessment and suppressed findings).
- `Findings:` — the batch group and decision group defined in this file's batch classification section when this file defines one; flat when this file defines no batch classification or grouping is inactive.
- `Summary:` — command-specific final counts.
- `Dependency scope:` — one line per file the run's judgment actually depended on: `{file}: {lines}` with 1-based closed line ranges (e.g. `120-180,300-320`), `all` when the judgment covered the whole file. In full runs, every read-only subagent reports this scope for the files it read and the main agent carries it over verbatim; in single-executor flows, the executor reports its own scope for the files it read. The main agent uses it when writing the validation cache (`--ranges`, see `framework/validation_cache.md` §Dependency Declaration). Targeted runs may omit it.
- `Next step:` — the concrete command to run next with its reason; `None` when nothing further is needed. Guidance: fixes applied → "fixes applied — re-run the target-appropriate re-check command (`validate@{target}:check-{n}`; unit targets also `verify@{target}:{keyword}` / `review@{target}:{keyword}`) to confirm"; all gates green → "if the design is finalized, run `promote@{target}`"; blocked → "awaiting your decision on {item}".

**Targeted runs:** end the report with the command's targeted note ("This was a targeted check — no cache was written. Run `{command}@{target}` for a complete ...") after the `Next step` line.
==ATOM_END:report_skeleton==

### Body format (review)

```
Architecture assessment:
  conclusion: acceptable | needs_attention | unacceptable
  module_boundaries: {assessment} — {basis}
  responsibility_organization: {assessment} — {basis}
  dependency_clarity: {assessment} — {basis}
  abstraction_level: {assessment} — {basis}
  extension_landing_points: {assessment} — {basis}
  engineering_patterns: {assessment} — {basis}
  gate_findings: {P0/P1 findings from Dimension 8, if any}

Suppressed by spec:
  {location} — {issue} → {suppression_reason}, {spec_ref}
```

**`{layer}` (skeleton header):** the layer of the spec the review is based on — `candidate`, falling back to `stable` when no candidate exists (§2 Pre-review Setup).

**Architecture assessment (Dimension 8):** The assessment reports the code structure of the reviewed surface per Dimension 8 in §4 below. Gate-level architectural defects (P0/P1) appear both in the assessment's `gate_findings` and in the Findings section; taste-level observations appear as P2/P3 findings only. A spec-recorded architectural decision with a conforming implementation is not re-questioned here.

**Conclusion mapping:** any Dimension 8 P0/P1 gate finding → `unacceptable`; P2/P3 only → `needs_attention`; none → `acceptable`. The conclusion does not change the Gate Rules table — the gate remains decided by P0/P1 findings.

### Findings section

```
Findings:
  Batch group (N items) — fix does not change runtime behavior, suggested for batch handling:
    - {location}: {one-line fix} (P3, based on: {evidence location})
    ...
  Decision group (M items) — decided one by one:
    [{severity}] {location} — {issue}
          spec_context: {design context, if any}
          recommendation: {fix suggestion}
    ...
```

When batch grouping is inactive (threshold not met), the Findings section uses the flat format:

```
Findings:
  [{severity}] {location} — {issue}
        spec_context: {design context, if any}
        recommendation: {fix suggestion}
  ...
```

### Severity check

```
Severity check:
  confirmed: N | adjusted: N
  - {location} — adjusted {Px} → {Py}, evidence: {file}, reason: {one line}
```

**PASS:** No P0 or P1 findings exist.
**FAIL:** One or more P0 or P1 findings exist — blocks promote.

**Summary:** the skeleton's `Summary` line carries `Total potential findings: N / Suppressed by spec: N` — the P0-P3 counts already appear in the header's `Key counts`.

The gate outcome is carried by the unified skeleton's `Blocking promote: yes | no` header line; the Gate Rules table below decides it.

## Gate Rules

| Condition | Result |
|-----------|--------|
| P0 or P1 findings exist | FAIL — blocks promote |
| Only P2 or P3 findings | PASS — does not block promote |
| No findings | PASS — does not block promote |

For promote gate: a cache written by a full run with PASS result satisfies the promote requirement. Targeted re-reviews never write a cache, so they are informational only.

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

**Dependency scope report:** In addition to findings, every sub-agent reports the read scope of its slice — for each file it read, the line ranges its review judgment actually depended on (1-based closed intervals; `all` when the assessment covered the whole file):

```
Dependency scope:
  {file}: {lines}   # "all" for whole-file judgments
```

Review judgments commonly cover whole files (a code quality assessment has no partial scope) — report `all` honestly in that case; the cache's `deps` then covers the whole file by design. The main agent carries this report over and uses it when writing the review cache (`--ranges`, see `framework/spec_review_checklist.md` §8); the declared ranges must cover every region the review judgment depended on, including called functions and referenced structures.
==ATOM_END:spec_review_standard==

## 7. Cross-Check

### 7.1 Purpose

After collection, before output — validate that each finding holds in the full codebase and full document set, not just in the local review context.

Every review run needs this:
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
   - Include `mode: full`, severity counts, `blocking`, file hashes and dependency CIDs
   - Include full findings body (cannot be omitted — required for promote gate detail)

Full runs always write cache regardless of pass/fail. Targeted runs (`:{keyword}`) never write a cache, and a targeted run that finds P0/P1 deletes the existing cache — blocking findings at any granularity mean promote must not proceed. See `framework/validation_cache.md`.

**Cache record contract:** The cache records the file state read during the review run — it is independent of the user's decision outcome:

- Collect dependency evidence from the files read during the review run (same collection point as verify Step 8): for each file, run `specflowctl gate-evidence --file <path> --ranges <lines>` and record its `hash` + `deps` output in the cache's `files` entry. The `--ranges` values come from the sub-agents' `Dependency scope` reports (see §6); a file reported as `all` (or not reported) is declared without `--ranges` (whole file). The declared ranges must cover every region the review judgment depended on — including called functions and referenced structures (declare-heavy principle; see `framework/validation_cache.md` §Dependency Declaration)
- Write the cache before applying any user-approved fixes — presenting findings and waiting for the user's decision is presentation only and does not gate the cache write
- Any fix applied after the cache write makes the cache stale (promote's dependency check fails) — a full review re-run is required before promote. This is a promote-gate requirement enforced when the user triggers promote; it does not authorize automatic re-review after fixes. The agent must not re-run review on its own initiative (see HARD RULE 2 in `framework/concepts.md`). After a fix, the agent guides the user to a targeted re-review (`review@{unit}:{keyword}`) or a concrete full command and waits for the user to trigger it; the full re-run is triggered by the user when deciding to promote.

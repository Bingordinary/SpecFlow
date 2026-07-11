# Validate Checklist

## Overview

When an agent executes `spec_validate {unit}`, it uses the 9 checks defined in this file. This file is referenced by `framework/concepts.md` §3 — the agent reads this file at validate time, not proactively.

## Execution Rules

- **Subagent permissions:** may inspect file content, search text by pattern, and locate files by name pattern. Must NOT modify files, execute commands, or delegate to other agents.
- Each check reports **PASS** or **FAIL** with a reason.
- On FAIL, the agent must identify **which information sources contradict each other** (e.g., "spec body describes auto-retry logic but no acceptance item covers it"). The fix is recorded in the FAIL reason.
- Resolution types:
  - **fix_required** — A concrete repair can be made inside the current candidate spec.
  - **blocked** — Requires user input (unclear intent, missing decision, or external dependency). Stop and ask.

## Output Format

```
Validate result: PASS | FAIL (fix_required | blocked)
1. Structural integrity: PASS | FAIL — reason
2. Design soundness: PASS | FAIL — reason
3. Scope integrity: PASS | FAIL — reason
4. Intent consistency: PASS | FAIL — reason
5. Acceptance coverage: PASS | FAIL — reason
6. Affects-source validity: PASS | FAIL — reason
7. Replacement/repair integrity: PASS | FAIL — reason
8. Cross-unit consistency: PASS | FAIL — reason
9. Constraint alignment: PASS | FAIL — reason
Resolution: fix_required | blocked — next step
Summary: ...
```

---

## Check 1 — Structural integrity

**Purpose:** Verify the file is parseable and all required fields exist, as a prerequisite for all subsequent checks.

**Execution steps:**

1. Read `docs/specs/units/candidate/c_unit_{unit}.md`
2. Verify required frontmatter fields: `id`, `layer` (must be `"candidate"`), `version`, `unit_refs`, `rule_refs`
3. Verify `acceptance_item_set` exists with at least one item. Each item must have: `id`, `description`, `verification_type`, `verification_surface`, `implementation_surface`, `verification_method`, `pass_condition`, `not_runnable_yet`
4. Verify all `unit_refs` point to existing stable spec files (format `s_unit_{name}@version`)
5. Verify all `rule_refs` point to existing rule files (global or bound)
6. Verify any appendix files referenced in the spec body exist at the expected path

**PASS:** All format constraints satisfied

**FAIL:** Any missing field or reference to a non-existent file (fix_required)

**Check method:** Unidirectional existence check (the only check that does not cross-reference, as it is the prerequisite)

---

## Check 2 — Design soundness

**Purpose:** Evaluate whether the design itself is correct and reasonable — not just whether it is well-documented. The subagent must actively reason about the design, not passively verify documentation completeness.

**Execution steps:**

**Step 1 — Goal-means analysis**
- Read the unit's goal and scope
- For each major behavior described: does it demonstrably serve a stated goal? If a behavior cannot be traced to any goal → flag (possible over-engineering)
- Reversely: is the goal achievable by implementing all described behaviors? If implementing everything still does not meet the goal → flag (design gap)
- Check whether any behavior violates a stated non-goal (e.g., non-goal says "no multi-tenancy this round" but the behavior describes tenant isolation)

**Step 2 — Design rationale review**
- Does the spec explain **why** each key design decision was made? (e.g., "chose event-driven architecture because async decoupling is required, not because it is popular")
- If there are viable alternative approaches (sync vs async, push vs pull, strong vs eventual consistency), does the spec acknowledge them and explain why they were rejected?
- If a design choice is non-obvious and no rationale is given → FAIL (fix_required: add design decision record)

**Step 3 — Adversarial analysis (red-team)**
Actively search for design flaws by considering:

| Attack angle | Questions to ask |
|---|---|
| Dependency failure | What happens when a dependency returns an error, times out, or crashes? Is fallback or degradation defined? |
| Concurrency | Can concurrent requests cause race conditions, data corruption, or duplicate operations? Are locks, idempotency keys, or transaction boundaries needed? |
| Invalid input | Can malformed, malicious, or unexpected input bypass validation and cause undefined behavior? Are validation rules and rejection policies defined? |
| Boundary / limit | Is behavior defined under high load, large data volume, long-running execution, or resource exhaustion? Any resource leak risk? |
| Security | Is there unauthorized access, data leakage, or injection risk? Is auth enforced consistently at every entry point? |

If a plausible critical flaw is identified that the spec does not address → FAIL (blocked: needs user judgment on whether this is a design gap or intentional)

**Step 4 — Verdict**
- PASS: goal-means aligned, rationale documented, no critical flaws found
- FAIL: specific findings reported

**Check method:** Content reasoning + adversarial analysis (the subagent makes active engineering judgments)

---

## Check 3 — Scope integrity

**Purpose:** The declared scope, non-goals, and boundaries must be clear and internally self-consistent.

**Execution steps:**

1. Is the unit's goal and responsibility scope clearly stated?
2. Are first-round non-goals and boundaries explicitly defined?
3. Are dependencies, rule bindings, and ownership boundaries explicit?
4. **Self-consistency check:**
   - Do the goals and described behaviors agree? (goal description scope matches behavior scope)
   - Do non-goals conflict with any described behavior? (non-goal says "not doing X" but behavior describes X)
   - Are the boundaries respected by the behavior descriptions? (e.g., boundary is "client-side validation only" but behavior describes server-side logic)

**PASS:** Scope is clear and self-consistent; no non-goal is violated

**FAIL:** Ambiguous scope, goal/non-goal contradiction, or boundary violation (fix_required)

**Check method:** Multi-field cross-reference (goal × non-goal × behaviors)

---

## Check 4 — Intent consistency

**Purpose:** The `candidate_intent` field and its related fields (`repair_basis`, `source_basis`, `evidence_appendix_ref`) must form a coherent whole.

**Execution steps:**

```
IF candidate_intent is present in frontmatter:         ← skipped for unit_new (no candidate_intent)

  IF candidate_intent == "change":
    - repair_basis must be ABSENT (change may not claim repair)
    - source_basis must exist and be consistent with the design's relationship to existing code
        - behavior depends on existing implementation → existing_implementation or mixed
        - behavior replaces existing without using it as truth → replacement
        - completely new design → new_design

  IF candidate_intent == "repair":
    - repair_basis must be PRESENT, formatted as s_unit_{unit}@<version>
    - source_basis must be new_design
    - evidence_appendix_ref must be none
    - Repair Scope section must exist (details validated in Check 7)

IF source_basis == "existing_implementation" OR "mixed":
    - evidence_appendix_ref must be PRESENT and not none

IF source_basis == "replacement":
    - evidence_appendix_ref may be none
    - At least one acceptance item must satisfy:
        verification_type == inspectable
        AND evidence_requirements includes old_code_deleted and no_remaining_refs
```

**PASS:** All intent fields form a coherent whole

**FAIL:** Contradiction found (e.g., repair without repair_basis, or existing_implementation without evidence_appendix_ref) → fix_required

**Check method:** Four-field cross-reference (candidate_intent × repair_basis × source_basis × evidence_appendix_ref)

---

## Check 5 — Acceptance coverage

**Purpose:** The behaviors described in the spec body and the acceptance items must cover each other bidirectionally.

**Execution steps:**

**Forward coverage (spec body → acceptance items):**
1. Extract all key behaviors from the spec body (main flow, protocols, error handling, state transitions, etc.)
2. Does each key behavior have at least one corresponding acceptance item?
3. If a behavior is described but has no acceptance item → flag (possible untested behavior)

**Reverse coverage (acceptance items → spec body):**
4. Is each acceptance item's `pass_condition` specific and verifiable?
   - PASS: "Returns HTTP 201 with id and created_at in response body" — specific
   - FAIL: "Behavior works normally" — not verifiable
   - FAIL: "Demo behavior passes the declared checks" — circular reference
5. Can each acceptance item be traced to a behavior described in the spec body?
6. If an acceptance item describes behavior not found in the spec body → flag (possible undocumented scope leak)

**Coverage completeness:**
7. Do all acceptance items' pass_conditions, taken together, prove the design goal is met?
   - If critical verification points are missing (e.g., only happy path tested, no error path) → flag

**PASS:** Bidirectional coverage verified, all pass_conditions are verifiable

**FAIL:** Uncovered behavior or orphaned acceptance item (fix_required); design goal cannot be verified → blocked (needs user confirmation on whether acceptance criteria meet the goal)

**Check method:** Spec body × acceptance item set bidirectional cross-reference

---

## Check 6 — Affects-source validity

**Purpose:** Each acceptance item's `affects` declarations must be consistent with the spec's formal references. Evidence appendix content must be consistent with the claimed `source_basis`.

**Execution steps:**

1. For each acceptance item, verify:

```
affects.rules:
  - Each rule must be either a global rule or listed in frontmatter rule_refs
  - Referencing a non-existent or undeclared rule → FAIL

affects.dependencies:
  - Each dependency must appear in frontmatter unit_refs
  - Referencing an undeclared dependency → FAIL

affects.files:
  - Each file path must point to an existing file in the project
  - Path should belong to this unit's ownership (if ownership is clearly defined)

affects.appendices:
  - Each appendix must exist and belong to this unit
  - The appendix frontmatter must declare the correct unit and layer
```

2. If `evidence_appendix_ref` is not `none`:
```
- Read the referenced appendix file
- Its content must be semantically consistent with the declared source_basis:
    - existing_implementation → appendix must record actually observed implementation behavior
    - mixed → appendix must clearly distinguish which parts are existing and which are new design
- Appendix content must not be only background, motivation, principles, or patch notes
  (must be directly readable truth about the current design)
- If content contradicts the declared source_basis → FAIL (fix_required)
```

**PASS:** All affects declarations are valid, evidence appendix is semantically consistent

**FAIL:** Reference inconsistency or appendix content contradicts declaration (fix_required)

**Check method:** affects.* × frontmatter refs cross-reference + source_basis × appendix content semantic cross-reference

---

## Check 7 — Replacement/repair integrity

**Purpose:** Candidates with specific intents must satisfy additional quality rules.

**Execution steps:**

**IF candidate_intent == "repair":**
```
1. Must have a Repair Scope section containing:
   - List of acceptance item IDs being restored
   - Observed deviations from expected behavior
   - Expected implementation-side changes
   - Required verification evidence
2. Repair Scope must not redefine behavior truth:
   - Must not modify protocol contracts
   - Must not modify field definitions
   - Must not modify ownership boundaries
   - Must not modify state machine semantics
   If any of the above is violated → FAIL (recommend switching to change intent)
3. Does the repair candidate actually restore the behavior of the repair_basis version?
   - Read the stable spec at repair_basis version (from git history or tag)
   - Verify the candidate's behavior matches that version's stable spec
```

**IF source_basis == "replacement":**
```
1. At least one acceptance item must have verification_type == inspectable
2. That item's evidence_requirements must include:
   - old_code_deleted
   - no_remaining_refs
```

**PASS:** Intent-specific integrity requirements satisfied

**FAIL:** Rule violation (fix_required) or repair exceeds permissible scope (recommend change instead)

**Check method:** Intent × acceptance item format cross-reference + stable version comparison

---

## Check 8 — Cross-unit consistency

**Purpose:** The candidate spec must not contradict related units (candidates and stables). Unacknowledged contract changes must be flagged.

**Execution steps:**

1. From `unit_refs`, get the list of dependency units
2. For each dependency unit:
   - If a candidate spec exists, read it and check for conflicting statements about shared protocols, data formats, or behavior
   - Read the stable spec and check whether this candidate changes any contract that the stable spec depends on
3. Specific checks:
```
   - Are API signatures compatible across all related units?
   - Are data formats (field names, types, enum values) consistent?
   - Is behavior semantics non-conflicting? (e.g., unit A assumes sync, unit B assumes async)
   - Does this candidate modify a contract that a dependency stable spec relies on?
     If yes → must be explicitly declared in the spec body
     If not declared → FAIL (blocked: needs user confirmation on downstream impact)
```

**PASS:** No contradictions across related units; acknowledged contract changes are declared

**FAIL:** Contradiction found (fix_required) or unacknowledged contract breakage (blocked)

**Check method:** Candidate × related candidates × related stables three-way cross-reference

---

## Check 9 — Constraint alignment

**Purpose:** The candidate design must not violate global constraints or bound rules.

**Execution steps:**

1. Read `docs/specs/system_constraints.md` if it exists
2. If frontmatter contains `system_constraints_ref`, verify its version matches the current constraints file version
3. Check each constraint against the candidate design:
```
   - Does any behavior conflict with a documented constraint?
   - If constraint says "all APIs must use HTTPS" — does the design describe HTTP?
   - If constraint says "synchronous calls are not supported" — does the design depend on sync calls?
```
4. Read each bound rule listed in `rule_refs`
5. Check the candidate design against each bound rule:
```
   - Is every "must not" prohibition respected?
   - Is every "must" requirement satisfied?
```

**PASS:** Candidate is compatible with all constraints and bound rules

**FAIL:** Constraint or rule violation found (fix_required)

**Check method:** Candidate × system_constraints × bound rules three-way cross-reference

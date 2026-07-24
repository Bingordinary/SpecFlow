# Validate Checklist

## Overview

When an agent executes `spec_validate {unit}`, it uses the 8 checks defined in this file. This file is referenced by `framework/concepts.md` §3 — the agent reads this file at validate time, not proactively.

## Mode Selection

Before executing, read `framework/verification_scope.md` to determine the current scope mode from the trigger phrase. The mode determines which subset of checks to run:

| Trigger | Mode | What to execute |
|---------|------|-----------------|
| `spec_validate {unit}` | scoped (default) | Git-aware: `git diff HEAD` on spec file → map changes to check(s) → run with dependency handling. See `framework/verification_scope.md` §Scoped Validate. |
| `spec_validate {unit}:check-{n}` | scoped | Single check `{n}` only |
| `spec_validate {unit}:{keyword}` | scoped | Match keyword to check name (e.g., "design" → Check 2, "coverage" → Check 5a, "drift" → Check 5c, "conflict" → Check 5d) |
| `spec_validate {unit}:full` | full | All 8 checks + cross-check (see `framework/verification_scope.md` §Cross-check for details) |

**Output:** prefix the result with `Mode: scoped` or `Mode: full` and the specific scope (e.g., `Scope: check-1 (structural integrity)`). For scoped results, append a note: "This is not a full validation. Only check {n} was executed. Run `spec_validate {unit}:full` for complete validation."

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
1. Structural integrity: PASS | WARNING | FAIL — reason
2. Design soundness: PASS | FAIL — reason
3. Scope integrity: PASS | FAIL — reason
4. Evidence-driven vs design-driven consistency: PASS | FAIL — reason
5. Acceptance coverage & correctness: PASS | FAIL — reason
  5a. Coverage completeness: PASS | FAIL — reason
  5b. Content alignment: PASS | FAIL — reason
  5c. Change drift: PASS | WARNING | FAIL — reason
  5d. Internal consistency: PASS | FAIL — reason
6. Affects-source validity: PASS | FAIL — reason
7. Cross-unit consistency: PASS | FAIL — reason
8. Constraint alignment: PASS | FAIL — reason
Resolution: fix_required | blocked — next step
Summary: ...
```

---

## Check 1 — Structural integrity

**Purpose:** Verify the file is parseable and all required fields exist, as a prerequisite for all subsequent checks.

**Execution steps:**

1. Read `docs/specs/units/candidate/unit_{unit}.md`
2. Verify required frontmatter fields: `id`, `layer` (must be `"candidate"`), `version`, `unit_refs`, `rule_refs`
3. Verify `acceptance_item_set` exists with at least one item. Each item must have: `id`, `description`, `verification_type`, `verification_surface`, `implementation_surface`, `verification_method`, `pass_condition`, `runnable`
4. Verify all `unit_refs` point to existing spec files (bare name, e.g. `agent`). Resolve by searching candidate directory first (`unit_{name}.md`), then fall back to stable (`unit_{name}.md`).
5. Verify all `rule_refs` point to existing rule files (global or bound)
6. Verify any appendix files referenced in the spec body exist at the expected path
7. **Prose-path hygiene check (WARNING):** Verify that prose sections (Description, Responsibility, and any other narrative sections) do not contain code file paths:
   - Scan narrative text for strings matching source-code file path patterns (backtick-enclosed or bare strings containing `/` and a source-code file extension like `.go`, `.ts`, `.py`, `.js`, `.java`, `.rs`, `.cs`)
   - Exclusions:
     - Structured fields: `implementation_surface`, `affects.files` (intentional)
     - Spec/governance system paths: `docs/specs/`, `framework/` (describe the spec system itself)
     - File paths inside code-block examples serving as illustrations
   - If code file paths are found in prose → WARNING with quoted path, section name, and line reference

**PASS:** All format constraints satisfied

**WARNING (step 7):** Code file paths detected in prose sections — relocate to `implementation_surface` or `affects.files`, or convert to a spec/governance path reference

**FAIL:** Any missing field or reference to a non-existent file (fix_required)

**Check method:** Unidirectional existence check (the only check that does not cross-reference, as it is the prerequisite)

**Communication note:** When suggesting Check 1 to a user, describe it as "structural integrity — verifies file structure and reference existence without evaluating design quality."

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

## Check 4 — Evidence-driven vs design-driven consistency

**Purpose:** Verify consistency between `evidence_appendix_ref` and the spec body. If an evidence appendix is declared, it must contain observed implementation behavior; if absent, the design must be self-contained or declarative of its replacement nature.

**Execution steps:**

```
IF evidence_appendix_ref is PRESENT and not none:
  → The design is based on observed implementation (evidence-driven)
  → The appendix content must record actually observed implementation behavior
     (semantic consistency verified in Check 6)

IF evidence_appendix_ref is ABSENT or none:
  → The design is design-driven (new concept or pure design change)
  → IF any acceptance item has verification_type == inspectable
       AND evidence_requirements includes old_code_deleted and no_remaining_refs:
       → This candidate is a replacement
       → Verify old code retirement separately (unit_verify_checklist Step 4)
```

**PASS:** evidence_appendix_ref is consistent with the spec body

**FAIL:** Contradiction found (fix_required)

**Check method:** evidence_appendix_ref × acceptance item attributes

---

## Check 5 — Acceptance coverage & correctness

**Purpose:** The spec body and acceptance items must cover each other bidirectionally, match semantically (5b), stay current as the body evolves (5c), and contain no internal contradictions (5d).

**Execution steps:**

### Sub-check 5a — Coverage completeness

**Purpose:** Every key behavior in the spec body must have at least one corresponding acceptance item, and the item's surface fields must be consistent with the behavior type. Enhanced from the original forward coverage check.

**Execution steps:**

1. Extract all key behaviors from the spec body (main flow, protocols, error handling, state transitions, etc.)
2. For each behavior, verify at least one acceptance item covers it
3. For each covered behavior, verify the item's `implementation_surface` and `verification_surface` are consistent with the behavior's nature (e.g., REST API behavior should have surface `api`, not `db`)
4. If a behavior has no acceptance item → flag (possible untested behavior)

**PASS:** All behaviors have corresponding items with appropriate surface fields

**FAIL:** Uncovered behavior or surface type mismatch (fix_required)

**Mode:**
| Mode | Scope |
|---|---|
| Scoped | Only body sections changed in `git diff HEAD` |
| Full | Entire spec body |

**Check method:** Spec body × acceptance item set unidirectional cross-reference (body → items)

---

### Sub-check 5b — Content alignment (NEW)

**Purpose:** For each behavior–item pair, detect semantic contradictions between the spec body description and the item's `pass_condition`. This catches body edits that invalidate item content — whether from recent changes or historical drift.

**Execution steps:**

1. For each behavior in the spec body, identify its corresponding acceptance item(s)
2. Read both the body description text and the item's `pass_condition` text
3. Apply natural language reasoning to identify semantic contradictions:

| Contradiction type | Body says | Item says |
|---|---|---|
| Value conflict | "timeout: 30s" | "respond within 5s" |
| Behavior conflict | "login accepts email+password" | "return error when password provided" |
| Scope conflict | "supports OAuth and API Key" | "only validates API Key" |
| Direction conflict | "increment counter" | "decrement counter" |

4. For each contradiction, report with **exact quotes** from both sources and a reasoning statement

**PASS:** No contradictions found

**FAIL:** One or more contradictions found, with quoted evidence (fix_required)

**Mode:**
| Mode | Scope |
|---|---|
| Scoped | Only body sections changed in `git diff HEAD`, and their corresponding items |
| Full | All behavior–item pairs |

**Check method:** Spec body × acceptance item pass_condition — semantic cross-reference with quoted evidence

---

### Sub-check 5c — Change drift detection (NEW)

**Purpose:** Detect when spec body sections were modified but their corresponding acceptance items were not updated proportionally. This serves as a real-time warning for "forgot to update item" scenarios. It is an auxiliary layer to 5b — 5b catches the resulting contradiction regardless, while 5c catches it at edit time.

**Execution steps:**

#### Scoped mode:
1. Run `git diff HEAD` on the spec file
2. Identify which body sections were modified (additions, changes, deletions)
3. For each modified section, find the corresponding acceptance item ID(s)
4. Check if those item(s) were also modified in the same diff
5. If body section changed but item did not → WARNING with diff evidence

#### Full mode:
1. If a stable predecessor spec exists, compare current body against stable version
2. Identify body sections that differ significantly
3. Check if corresponding items also differ
4. If body differs but item unchanged → WARNING

**PASS:** No drift detected (all modified body sections have corresponding item changes)

**WARNING:** Potential drift found — body changed but items unchanged. Report with diff excerpts. Resolution: review item and update if needed.

**Output level is WARNING, not FAIL.** Not all body changes require item changes (e.g., formatting, comments, clarification). The agent reports drift as potential issues for human review.

**Mode:**
| Mode | Baseline |
|---|---|
| Scoped | `git diff HEAD` against working tree |
| Full | Stable predecessor spec (if exists); skip if no predecessor |

**Check method:** Git diff × acceptance item modification check — temporal cross-reference

---

### Sub-check 5d — Internal consistency (NEW)

**Purpose:** Detect contradictions between acceptance items within the same spec. Two items targeting the same verification surface must not describe contradictory requirements.

**Execution steps:**

1. **Group by `verification_surface`:** items sharing the same surface are likely checking the same endpoint/module
   - Compare their `pass_condition` texts for contradictions
   - Example: item A says "returns 201", item B says "returns 200" for same API → conflict

2. **Group by `affects.files`:** items referencing the same implementation file
   - Check if their behavioral descriptions conflict
   - Example: item A says "write requires auth", item B says "write is public" → conflict

3. **Cross-item value/assumption check:**
   - Detect obvious numeric contradictions (one says <100ms latency, another says <5s for same operation)
   - Detect logical contradictions (one says "enabled by default", another says "opt-in only")

**PASS:** No conflicts found

**FAIL:** One or more conflicts found, with quoted evidence from both items (fix_required)

**Mode:**
| Mode | Scope |
|---|---|
| Scoped | Only items changed in `git diff HEAD` (check new/modified items against all existing items) |
| Full | All items against all items |

**Check method:** Cross-item cross-reference by verification_surface and affects.files

---

## Check 6 — Affects-source validity

**Purpose:** Each acceptance item's `affects` declarations must be consistent with the spec's formal references. Evidence appendix content must be structurally sound and semantically meaningful.

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
- Its content must record actually observed implementation behavior
  (not only background, motivation, principles, or patch notes)
- If the appendix describes existing implementation behavior mixed with new design parts,
  it must clearly distinguish which parts are existing and which are new
- If content is only background or patch notes → FAIL (fix_required)
```

**PASS:** All affects declarations are valid, evidence appendix is semantically consistent

**FAIL:** Reference inconsistency or appendix content contradicts declaration (fix_required)

**Check method:** affects.* × frontmatter refs cross-reference + appendix content semantic assessment

---

## Check 7 — Cross-unit consistency

**Purpose:** The candidate spec must not contradict related units (candidates and stables). Unacknowledged contract changes must be flagged.

**Execution steps:**

1. From `unit_refs`, get the list of dependency units (format `{name}@version`, resolved candidate-first per Check 1)
2. For each dependency unit:
   - Candidate spec takes priority — read it and check for conflicting statements about shared protocols, data formats, or behavior
   - Also read the stable spec and check whether this candidate changes any contract that the stable spec depends on — skip the stable contract check if the dependency's candidate spec already reflects the change
3. Specific checks:
```
   - Are API signatures compatible across all related units?
   - Are data formats (field names, types, enum values) consistent?
   - Is behavior semantics non-conflicting? (e.g., unit A assumes sync, unit B assumes async)
   - Does this candidate modify a contract that a dependency stable spec relies on?
     If yes, and the dependency's candidate spec does not already reflect the same change
       → the change must be explicitly declared in this candidate's spec body
       If not declared → FAIL (blocked: needs user confirmation on downstream impact)
```

**PASS:** No contradictions across related units; acknowledged contract changes are declared

**FAIL:** Contradiction found (fix_required) or unacknowledged contract breakage (blocked)

**Check method:** Candidate × related candidates × related stables three-way cross-reference

---

## Check 8 — Constraint alignment

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

---

## Resolution Protocol

When one or more checks FAIL after all checks complete, follow the resolution protocol.

### Summary format

```
────────────────────────────────────────────────────
Mode: scoped | full
Validate result: FAIL
Findings:
  1. {check-name}: FAIL — {reason} (fix_required | blocked)
  2. {check-name}: FAIL — {reason} (fix_required | blocked)
  ...
────────────────────────────────────────────────────
```

==ATOM_BEGIN:resolution_protocol==
### Resolution entry

After presenting the summary, offer the user a choice:

```
Found {N} finding(s). How would you like to proceed?
  [1] One by one — explain each, then decide
  [2] Batch — I will give direction for all at once
  [3] Skip — I will handle these separately
```

- **[1] One-by-one** → Interactive Resolution Protocol (§Interactive Resolution Protocol)
- **[2] Batch** → record user's consolidated verdict; no per-finding dialogue
- **[3] Skip** → report findings without resolving; user takes over

### Interactive Resolution Protocol

When the user chooses one-by-one mode, present each finding sequentially.
Resolve one before showing the next.

```
═══════════════════════════════════════════════════
Finding {n} of {N}

Finding: {description}
Suggested direction: {direction}
───────────────────────────────────────────────────
  [1] Agree — apply this direction
  [2] Disagree — specify alternative
  [3] Discuss — I have questions about this one
───────────────────────────────────────────────────
```

On **[3] Discuss**: enter a free-form dialogue. Record a direction when the user states it clearly.

**Per-finding rule:** record the agreed direction before moving to the next finding.

**Completion:** after all findings are resolved, present a completion summary:

```
───────────────────────────────────────────────────
All {N} findings resolved:
  {finding.1}: {direction} — {next step}
  {finding.2}: {direction} — {next step}
  ...
───────────────────────────────────────────────────
```
==ATOM_END:resolution_protocol==

### Validate-specific notes

- **Cascade awareness:** fixing one check may affect others. After a fix is applied, inform the user: "Fix applied. Some remaining checks may be affected by this change."
- **Blocked items** (resolution_type: blocked) require user input — skip to next finding without suggesting a fix.

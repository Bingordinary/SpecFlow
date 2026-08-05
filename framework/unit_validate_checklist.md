# Validate Checklist

## Overview

When an agent executes `validate@ {unit}`, it uses the 8 checks defined in this file. This file is referenced by `framework/concepts.md` §3 — the agent reads this file at validate time, not proactively.

## Prerequisite — Read all unit files

Before executing any check, the agent must read the complete unit content:

1. Read the main spec: `docs/specs/units/candidate/unit_{unit}.md`
2. Glob all candidate appendix files: `docs/specs/units/candidate/appendix/unit_{unit}_*.md`
3. For each appendix, read its frontmatter. Skip files with `status: exempt`.
4. Read the content of every non-exempt appendix file.

The unit's complete spec is the union of the main spec and all non-exempt appendix files. All checks that follow operate on this union.

## Mode Selection

| Trigger | Mode | What to execute |
|---------|------|-----------------|
| `validate@ {unit}` | full (default) | All 8 checks + cross-check. Quality checks are holistic — always runs full. |
| `validate@ {unit}:check-{n}` | scoped | Single check `{n}` only. User explicitly chooses focus. |
| `validate@ {unit}:{keyword}` | scoped | Match keyword to check name (e.g., "design" → Check 2, "scope" → Check 3). User explicitly chooses focus. |
| `validate@ {unit}:full` | full | All 8 checks + cross-check. Explicit equivalent of default. |

**Output:** Prefix with `Mode: scoped` or `Mode: full`. For scoped: append note "Only check(s) {n} were executed. This is not a full validation. Run `validate@ {unit}:full` for complete validation."

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
Failed checks: N / Total findings: M / Advisory findings: K
1. Structural integrity: PASS | WARNING | FAIL — reason
2. Design soundness: PASS | FAIL — reason
3. Scope integrity: PASS | FAIL — reason
4. Evidence-driven vs design-driven consistency: PASS | FAIL — reason
5. Acceptance coverage & correctness: PASS | FAIL — reason
  5a. Coverage completeness: PASS | WARNING | FAIL — reason
  5b. Content alignment: PASS | FAIL — reason
  5c. Internal consistency: PASS | FAIL — reason
6. Affects-source validity: PASS | FAIL — reason
7. Cross-unit consistency: PASS | FAIL — reason
8. Constraint alignment: PASS | FAIL — reason
Resolution: fix_required | blocked — next step
Summary: ...
```

**Counting rules:**
- `Failed checks` is the number of FAIL checks among executed checks. WARNING is not a failed check.
- `Total findings` is the total number of distinct findings across all FAIL checks. In scoped mode, only executed checks are counted.
- `Advisory findings` (Check 2 Step 4 taste-level P2/P3) are presented on their check line's reason and counted separately as `Advisory findings: K`. They are never counted in `Total findings` and never affect `Failed checks`.
- The same counts are reused in the Present Findings summary (`Total findings` = batch group items + decision group items).

**Multi-finding enumeration:** When a FAIL reason contains multiple distinct findings, list each finding as an indented numbered sub-line under the check line. Each sub-line carries a location reference (the contradicting information sources, per Execution Rules), the finding statement, and its resolution type:

```
5a. Coverage completeness: FAIL — 3 findings
  5a-1. {location} — {finding} (fix_required)
  5a-2. {location} — {finding} (fix_required)
  5a-3. {location} — {finding} (blocked)
```

A check with a single finding keeps the existing one-line reason format.

When findings mix resolution types (within one check or across checks), the output `Resolution` line is `blocked` if any finding is `blocked` — a blocked finding stops the flow and requires user input per Execution Rules; otherwise it is `fix_required`.

---

## Check 1 — Structural integrity

**Purpose:** Verify the file is parseable and all required fields exist, as a prerequisite for all subsequent checks.

**Execution steps:**

1. Read `docs/specs/units/candidate/unit_{unit}.md` and all non-exempt appendix files (see Prerequisite)
2. Verify required frontmatter fields: `id`, `layer` (must be `"candidate"`), `version`, `unit_refs`, `rule_refs`
3. Verify `acceptance_item_set` exists with at least one item. Each item must have: `id`, `description`, `verification_type`, `verification_surface`, `implementation_surface`, `verification_method`, `pass_condition`, `runnable`
4. Verify all `unit_refs` point to existing spec files (bare name, e.g. `agent`). Resolve by searching candidate directory first (`unit_{name}.md`), then fall back to stable (`unit_{name}.md`).
5. Verify all `rule_refs` point to existing rule files (global or bound)
6. Verify any appendix files referenced in the spec body exist at the expected path
7. **Prose-path hygiene check (WARNING):** Verify that prose sections (Description, Responsibility, and any other narrative sections) do not contain code file paths:
   - Scan narrative text for strings matching source-code file path patterns (backtick-enclosed or bare strings containing `/` and a source-code file extension like `.go`, `.ts`, `.py`, `.js`, `.java`, `.rs`, `.cs`)
   - Exclusions:
     - Structured fields: `implementation_surface`, `affects.files` (intentional)
     - Framework governance paths (`framework/`) and validation cache paths (`docs/specs/meta/`) — describe the governance system itself
     - File paths inside code-block examples serving as illustrations
   - If code file paths are found in prose → WARNING with quoted path, section name, and line reference
8. **Appendix frontmatter check:** For each non-exempt appendix file, verify:
   - `unit` frontmatter field matches current unit name
   - `layer` frontmatter field is `"candidate"`
9. **Appendix path check:** Verify each appendix file's path matches the convention: `docs/specs/units/candidate/appendix/unit_{unit}_{name}.md`
10. **Layer-prefix path check (FAIL):** Scan the main spec body and every non-exempt appendix for layer-prefixed spec paths that break after promote or mispoint during an active candidate round. Unlike step 7, there is no code-block exemption: paths inside code-block examples are flagged the same as prose.
    - Absolute forms: `docs/specs/units/candidate/`, `docs/specs/units/stable/`, `docs/specs/rules/candidate/`, `docs/specs/rules/stable/`
    - Relative forms: `candidate/`, `stable/`
    - Reference appendix files and other specs by concept name or file name (e.g. `unit_auth_account_token_claims`) instead — appendix file names do not encode layer, so the reference stays valid before and after promote
    - Structured field exemption: `implementation_surface`, `affects.files`, `affects.appendices`, `affects.dependencies` values may contain a stable-layer spec path (it stays valid after promote); a candidate-layer spec path in any structured field is invalid (promote deletes candidate files)
    - If layer-prefixed spec paths are found → FAIL with quoted path, section name, and line reference

**PASS:** All format constraints satisfied

**WARNING (step 7):** Code file paths detected in prose sections — relocate to `implementation_surface` or `affects.files`, or convert to a concept name reference

**FAIL:** Any missing field, reference to a non-existent file, or layer-prefixed spec path in prose or structured fields (fix_required)

**Check method:** Unidirectional existence check (the only check that does not cross-reference, as it is the prerequisite)

**Communication note:** When suggesting Check 1 to a user, describe it as "structural integrity — verifies file structure and reference existence without evaluating design quality."

---

## Check 2 — Design soundness

**Purpose:** Evaluate whether the design itself is correct and reasonable — not just whether it is well-documented. The subagent must actively reason about the design, not passively verify documentation completeness. Appendix content describing design decisions, API contracts, component trees, or data types is part of the unit's design and must be included in this analysis.

**Execution steps:**

**Step 1 — Goal-means analysis**
- Read the unit's goal and scope from the main spec AND appendix files
- For each major behavior described (in main spec or appendices): does it demonstrably serve a stated goal? If a behavior cannot be traced to any goal → flag (possible over-engineering)
- Reversely: is the goal achievable by implementing all described behaviors? If implementing everything still does not meet the goal → flag (design gap)
- Check whether any behavior violates a stated non-goal (e.g., non-goal says "no multi-tenancy this round" but the behavior describes tenant isolation)
- **Proportionality check:** Is the design complexity proportional to the stated goal? If the same goal could be achieved with significantly less design surface area → flag (possible over-engineering)

**Step 2 — Design rationale review**
- **Evidence-driven precondition:** Read the spec frontmatter's `evidence_appendix_ref` field.
  - If `evidence_appendix_ref` is PRESENT and not `none` → the spec is evidence-driven (design records existing implementation). The code behavior itself constitutes the design rationale. **Skip** the rationale review below. Report "Step 2: N/A (evidence-driven — rationale is implicit in existing code)" and proceed to Step 3.
  - If `evidence_appendix_ref` is ABSENT or `none` → the spec is design-driven. Execute the rationale review below.
- Does the spec explain **why** each key design decision was made? (e.g., "chose event-driven architecture because async decoupling is required, not because it is popular")
- If there are viable alternative approaches (sync vs async, push vs pull, strong vs eventual consistency), does the spec acknowledge them and explain why they were rejected?
- If a design choice is non-obvious and no rationale is given → FAIL (fix_required: add design decision record)

**Step 3 — Adversarial analysis (red-team)**
Actively search for design flaws in both main spec and appendix content. Read appendix descriptions of API contracts, data formats, state machines, error handling, and include them in each attack angle:

| Attack angle | Questions to ask |
|---|---|
| Dependency failure | What happens when a dependency returns an error, times out, or crashes? Is fallback or degradation defined? |
| Concurrency | Can concurrent requests cause race conditions, data corruption, or duplicate operations? Are locks, idempotency keys, or transaction boundaries needed? |
| Invalid input | Can malformed, malicious, or unexpected input bypass validation and cause undefined behavior? Are validation rules and rejection policies defined? |
| Boundary / limit | Is behavior defined under high load, large data volume, long-running execution, or resource exhaustion? Any resource leak risk? |
| Security | Is there unauthorized access, data leakage, or injection risk? Is auth enforced consistently at every entry point? |

If a plausible critical flaw is identified that the spec does not address → FAIL (blocked: needs user judgment on whether this is a design gap or intentional)

**Step 4 — Design decision taste level**
Assess only the design decisions the spec has actually recorded (design decision records, architecture descriptions, component trees, data types in main spec and appendices). Evaluate taste-level quality of those recorded decisions:
- Are module/component boundaries cut at the natural seams of the stated behavior domains?
- Does each recorded responsibility stay single-purpose?
- Are extension landing points explicit where the spec records future-work expectations?
- Do the recorded decisions follow the repository's established engineering patterns?

Output P2/P3 advisory findings — these do NOT affect the PASS/FAIL verdict and do not block promote. If the spec records no design decisions (no architecture section, no design decision records, and no appendix design content) → report "Step 4: N/A (no recorded design decisions)" and proceed. This step must not invent architecture the spec does not record; it evaluates only what is written. Objective design defects remain Step 3's territory.

**Step 5 — Verdict**
- PASS: goal-means aligned, rationale documented (or N/A when evidence-driven), no critical flaws found
- FAIL: specific findings reported

**Check method:** Content reasoning + adversarial analysis + taste-level assessment (the subagent makes active engineering judgments)

---

## Check 3 — Scope integrity

**Purpose:** The declared scope, non-goals, and boundaries must be clear and internally self-consistent.

**Execution steps:**

1. Is the unit's goal and responsibility scope clearly stated?
2. Are first-round non-goals and boundaries explicitly defined?
3. Are dependencies, rule bindings, and ownership boundaries explicit?
4. **Self-consistency check (main spec):**
   - Do the goals and described behaviors agree? (goal description scope matches behavior scope)
   - Do non-goals conflict with any described behavior? (non-goal says "not doing X" but behavior describes X)
   - Are the boundaries respected by the behavior descriptions? (e.g., boundary is "client-side validation only" but behavior describes server-side logic)
5. **Appendix scope check:** Verify that appendix content does not exceed the unit's declared scope. If an appendix describes behavior belonging to a different unit's responsibility → FAIL (fix_required: move content to the correct unit or declare scope expansion)

**PASS:** Scope is clear and self-consistent; no non-goal is violated; appendix content stays within unit scope

**FAIL:** Ambiguous scope, goal/non-goal contradiction, boundary violation, or out-of-scope appendix content (fix_required)

**Check method:** Multi-field cross-reference (goal × non-goal × behaviors × appendix content)

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

**Purpose:** The spec body and acceptance items must cover each other bidirectionally, match semantically (5b), and contain no internal contradictions (5c). Acceptance items must also have falsifiable pass_conditions (5e), actionable descriptions (5f), and coupled pass_condition/description pairs that add value (5g).

**Execution steps:**

### Sub-check 5a — Coverage completeness

**Purpose:** Every behavior domain in the spec body and appendices must have at least one corresponding acceptance item, and the item's surface fields must be consistent with the behavior type. Granularity baseline: behavior domains as defined in `framework/spec_writing_guide.md` §Acceptance Item Granularity — one item = one behavior domain with its full scenario set (happy path + error paths + boundary cases). Enhanced from the original forward coverage check to a bidirectional check.

**Execution steps:**

1. **Coverage input source:** A behavior is covered when any acceptance item describes it in its `description` (Given/When/Then scenarios) OR constrains it in its `pass_condition`. The coverage judgment input is the union of `description` and `pass_condition` — a behavior constraint that already appears in some item's `pass_condition` counts as covered, consistent with sub-check 5g (which requires `pass_condition` to carry constraints beyond `description`).
2. Extract all behavior domains from the spec body (main flow, protocols, error handling, state transitions, etc.) at the granularity defined in `framework/spec_writing_guide.md` §Acceptance Item Granularity: group behavior variants around one behavior subject into one domain; do not split scenarios of the same domain into separate coverage requirements.
3. For each behavior domain, verify at least one acceptance item covers it (using the union input from step 1)
4. For each covered domain, verify the item's `implementation_surface` and `verification_surface` are consistent with the behavior's nature (e.g., REST API behavior should have surface `api`, not `db`)
5. If a behavior domain has no acceptance item → flag (possible untested behavior)
6. **Appendix behavior coverage check:** Extract all behavior domains, API contracts, data type definitions, and state machine transitions from appendix files. For each, verify there is at least one acceptance item in the main spec covering it. If an appendix describes content that has no corresponding acceptance item → WARNING (possible orphan behavior — documented in appendix but not verified). If appendix content directly contradicts an acceptance item (e.g., appendix says "timeout: 30s", item says "respond within 5s") → FAIL (fix_required)
7. **Over-splitting detection (reverse check):** If multiple acceptance items satisfy the same behavior domain judgment (same behavior subject + same `verification_surface` + same `implementation_surface` + same `verification_type`), they are merge candidates → WARNING recommending a merge into one item. Merge method: keep one item id, delete the rest — the surviving id's process evidence stays valid. Items differing in `verification_type` are legitimate splits, not merge candidates.

**PASS:** All behavior domains (main spec + appendices) have corresponding items with appropriate surface fields; no merge candidates found

**WARNING:** Orphan appendix behavior (documented but not verified) or merge candidates (over-split acceptance items)

**FAIL:** Uncovered behavior domain or surface type mismatch (fix_required); appendix-main spec contradiction (fix_required)

**Check method:** Spec body + appendices × acceptance item set bidirectional cross-reference (body → items for coverage; items → body for over-splitting detection)

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

**Check method:** Spec body × acceptance item pass_condition — semantic cross-reference with quoted evidence

---

### Sub-check 5c — Internal consistency (NEW)

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

**Check method:** Cross-item cross-reference by verification_surface and affects.files

---

### Sub-check 5d — Description format compliance

**Purpose:** Verify that each acceptance item with `verification_type: testable` uses Gherkin-style Given/When/Then format in its `description`, as required by `framework/spec_writing_guide.md` §Gherkin-style Description Convention.

**Execution steps:**

1. For each acceptance item in scope:
   - If `verification_type` is `testable`, read the `description` field
2. Check that the description contains at least one `Given`…`When`…`Then` sequence (case-insensitive pattern: lines starting with `Given`, `When`, `Then` in order)
3. Reject `.feature` file syntax (`Feature:`, `Scenario:`, `Scenario Outline:`, `Examples:`, `Background:`) — the Gherkin-style convention explicitly does not use these
4. If any testable item lacks the Given/When/Then pattern → FAIL with item ID and quoted description

**PASS:** All testable items use Gherkin-style description format

**FAIL:** One or more testable items have non-compliant description format (fix_required)

**Check method:** Acceptance item description × verification_type — format pattern check

---

### Sub-check 5e — Falsifiability (NEW)

**Purpose:** Every acceptance item's `pass_condition` must be falsifiable — there must exist a concrete, identifiable scenario where the implementation could fail it. An unfalsifiable pass_condition can be "satisfied" by any implementation, making the item meaningless as a quality gate.

**Execution steps:**

For each acceptance item:

1. Read `pass_condition`
2. Apply first-principles reasoning: "If the implementation were broken, would there be a way for this pass_condition to reveal it?"
3. Identify a specific failure scenario that would cause the pass_condition to FAIL:
   - Concrete: names a specific behavior change ("returns 200 instead of 201", "missing field X in response", "allows duplicate email registration")
   - Observable: the failure could be detected by reading code or test output
   - Distinct: the failure scenario is different from "the code doesn't exist" (that's structural, covered by Step 1)
4. If agent cannot identify a concrete failure scenario → the item is unfalsifiable
5. Write an explicit statement for each item: "This item would FAIL if [specific code behavior or condition] occurs."

**Reports:**

```
{item.id}: PASS
  - pass_condition: "Returns HTTP 201 with {id, email, created_at}"
  - Would FAIL if: missing created_at field, returns 200, returns 500 on valid input

{item.id}: FAIL — Unfalsifiable
  - pass_condition: "registration works correctly"
  - "works correctly" is a value judgment, not a verifiable condition.
    No concrete code change would cause this specific condition to FAIL.
  - fix_required: Replace with a specific, measurable pass_condition
```

**Edge cases:**
- Pass conditions referring to external systems or runtime constraints that are not statically verifiable → not automatically unfalsifiable. Agent records them as CANNOT_DETERMINE in verify but does not fail validate.

**PASS:** All items have an identifiable failure scenario

**FAIL:** One or more items are unfalsifiable

**Check method:** First-principles reasoning — agent must articulate a concrete failure mode and write an explicit "Would FAIL if" statement per item

---

### Sub-check 5f — Description actionability (NEW)

**Purpose:** For `verification_type: testable` items, the `description` must contain enough detail to derive specific test scenarios. A vague description produces vague tests — or leaves the agent to invent scenarios that don't reflect the author's intent.

**Execution steps:**

1. For each acceptance item with `verification_type: testable`:
   - Read `description`
   - Judge whether it contains enough information to derive:
     - Specific input values or conditions
     - Expected output or state change
     - At least one boundary or edge case
2. If the description is a single vague sentence with no scenario breakdown → FAIL
3. If the description is short but specific (e.g., "Returns 201 when valid email and password are provided") → PASS
4. If the description is long but purely narrative with no testable specifics → FAIL

**Reports:**

```
{item.id}: PASS
  - description: "User registers with email and password. Returns 201 with {id, email, created_at}. Returns 409 if email exists."
  - Verdict: Enough detail to derive happy path, conflict scenario

{item.id}: FAIL — Description too vague for test derivation
  - description: "User can register"
  - Verdict: No inputs, no expected outputs, no conditions.
  - fix_required: Expand description with Given/When/Then scenarios or specific pass conditions
```

**PASS:** All testable items have actionable descriptions

**FAIL:** One or more testable items lack sufficient descriptive detail for test derivation

**Check method:** Semantic assessment — agent judges whether description is specific enough to derive test inputs and expected outcomes

---

### Sub-check 5g — Pass condition / description coupling (NEW)

**Purpose:** The `pass_condition` must add specific, verifiable constraints beyond what the `description` already communicates. If the pass_condition merely rephrases the description in vaguer or equivalent terms, it contributes no value and the acceptance item cannot be meaningfully verified.

**Execution steps:**

For each acceptance item:

1. Read both `description` and `pass_condition`
2. Compare them semantically:
   - Does `pass_condition` reference specific values, status codes, field names, error types, state transitions, timeouts, or behavior variants that the `description` does not?
   - Is the `pass_condition` more abstract or vague than the `description`?
3. If the `pass_condition` is semantically equivalent to or vaguer than the `description` → FAIL

**Examples:**

```
FAIL — pass_condition adds nothing:
  description:  "User registers with email and password"
  pass_condition: "registration completes successfully"
  → "completes successfully" is a vague rephrase of "registers"

PASS — pass_condition adds specific constraints:
  description:  "User registers with email and password"
  pass_condition: "Returns 201 with {id, email, created_at}. Returns 409 if email exists. Email field must be normalized to lowercase."
  → Adds three verifiable constraints not present in description

PASS — pass_condition provides complementary information:
  description:  "System exports data as CSV"
  pass_condition: "File is valid CSV: comma-separated, quoted strings, header row matches schema fields"
  → Adds specific format criteria not in description
```

**PASS:** All pass_conditions add specific value beyond their descriptions

**FAIL:** One or more pass_conditions are vague rephrasings of their descriptions

**Check method:** Semantic cross-reference — agent compares information content of description vs pass_condition

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

3. **Appendix file path references:** For each non-exempt appendix, scan its content for code file path references (strings containing `/` and a source-code file extension). For each path found, verify it points to an existing file in the project. If any path does not exist → FAIL (fix_required: update or remove the invalid path reference)

**PASS:** All affects declarations are valid, evidence appendix is semantically consistent, appendix file path references exist

**FAIL:** Reference inconsistency, appendix content contradicts declaration, or appendix references non-existent file paths (fix_required)

**Check method:** affects.* × frontmatter refs cross-reference + appendix content semantic assessment

---

## Check 7 — Cross-unit consistency

**Purpose:** The candidate spec must not contradict related units (candidates and stables). Unacknowledged contract changes must be flagged.

**Execution steps:**

1. From `unit_refs`, get the list of dependency units (bare names, resolved candidate-first per Check 1)
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
4. **Appendix cross-unit check:** Include appendix content (API contracts, data type definitions, error codes, state machines) in the cross-unit comparison. If an appendix defines a contract, data format, or protocol that conflicts with another unit's spec → FAIL (fix_required: resolve the cross-unit inconsistency)

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
4. Read the stable global rule set (`docs/specs/rules/stable/g_rule_*.md`) and each bound rule listed in `rule_refs`. Stable global rules apply to every current-layer unit by default and are not repeated in `rule_refs` (see `framework/spec_writing_guide.md` §4). For the circular-dependency prohibition (`g_rule_repository_baseline.md` §6.1 item 4), glob all current-layer unit spec files (candidate and stable) and derive the dependency graph from their `unit_refs`. For the layer-order prohibition (§6.1 item 5), resolve the order from the rule's §5.1 recording and each unit's declared architecture layer from its spec truth; a `unit_refs` edge from a lower-layer unit to a higher-layer unit is a violation unless the unit truth records an exception (`rule_exceptions`). Units without a recorded architecture layer are not judged by this prohibition.
5. Check the candidate design against each global rule and each bound rule:
```
   - Is every "must not" prohibition respected?
   - Is every "must" requirement satisfied?
```
6. **Rule exception re-evaluation:** Read the candidate spec's frontmatter `rule_exceptions` field (see `framework/spec_writing_guide.md` §2). For every recorded exception, first verify its reference validity, then re-evaluate whether the exception still holds against the current implementation and the current rule version:
   - Referenced rule is neither a stable global rule nor a bound rule listed in this unit's `rule_refs`, or the reason is missing → FAIL (fix_required: correct or remove the invalid exception entry)
   - Exception no longer justified (architecture was rewritten, rule changed, or the reason expired) → FAIL (fix_required: report the exception for removal; the removal is applied only after user approval)
   - Exception still justified → keep it and state the re-examination verdict in this check's reason
7. **Appendix constraint check:** Include appendix design descriptions, API contracts, and behavior definitions in the constraint and bound rule checking. If appendix content describes behavior that violates a global constraint or bound rule → FAIL (fix_required: align appendix content with constraints)

**PASS:** Candidate (main spec + appendices) is compatible with all constraints and bound rules; all recorded rule exceptions are still justified

**FAIL:** Constraint or rule violation found in main spec or appendices (fix_required)

**Check method:** Candidate × system_constraints × bound rules three-way cross-reference

---

## Step 9 — Write validate cache (main agent)

After all 8 checks complete:

- **If all PASS:** write validate cache per `framework/validation_cache.md` format:
  - Create `docs/specs/meta/validation/unit/{name}/` directory if needed
  - Collect SHA-256 hashes of all files read during validation, including:
    - Main spec file
    - Every non-exempt appendix file
    - All referenced files (unit_refs, rule_refs, affects.files are already included)
  - Write `validate_result.md` with `result: pass`, `mode: full`, file hashes
  - Exception: when triggered by `:check-{n}` or `:{keyword}`, write `mode: scoped` with `scoped_check: "{n}"`

- **If any FAIL:** delete existing `validate_result.md` if present. Do not write cache. Proceed to Present Findings.

---

## Present Findings

Advisory findings from Check 2 Step 4 are presented for awareness only — they enter neither the batch group nor the decision group, need no decision, and do not block the flow. They are presented on their check line's reason even when all checks PASS.

### Batch classification (validate)

When FAIL items exist, the main agent classifies each finding into a **batch group** or a **decision group** before presenting. Classification uses each FAIL's resolution type (fix_required / blocked) and the check definition — it adds no new analysis; for batch group candidates it only runs lightweight assertion re-verification (see Assertion re-verification below).

**Batch group eligibility:** A finding enters the batch group only if its fix is fully determined by an objective standard — no interpretation of design intent is required. The batch group is limited to these fix types:
- Check 1: missing required frontmatter fields (standard: the required fields list)
- Check 1: unit_refs / rule_refs / appendix references to non-existent files (standard: file existence)
- Check 1: appendix path or naming not following the convention (standard: the path convention)
- Check 5d: testable item description missing Given/When/Then (standard: the Gherkin-style convention in `framework/spec_writing_guide.md`)

All other FAIL findings — including 5e/5f/5g content rewrites, Checks 2/3/4/6/7/8, and every blocked item — go to the decision group. **Blocked items always go to the decision group.**

No activation threshold: findings are aggregated at check level rather than presented flat per item, so splitting out the batch group adds constant cost and always reduces the decisions the user must make — there is no over-splitting scenario. The batch group is inherently limited by the fix-type list above.

==ATOM_BEGIN:batch_findings_mechanism==
**Assertion re-verification:** After eligibility passes, the main agent re-verifies each batch group candidate's core assertion with 1-2 deterministic checks:
- Sub-agent claims "X is missing" → confirm X is really absent from the cited location (read the file, check existence, or grep as appropriate)
- Sub-agent claims "the cited source states X explicitly" → read the cited line, confirm X is really written there, without vague wording ("or", "suggested", alternatives)
- Sub-agent claims "the correct pattern exists elsewhere" → confirm that reference really exists

Re-verification failure → item moves to the decision group. This runs before execution because a post-execution check re-run cannot detect a wrong-direction fix — once the documents are made consistent, the re-run passes and the error is cemented.

**Execution boundary:** Classification is presentation only — it is not an authorization to act. Nothing is implemented until the user explicitly agrees to the whole batch group. The user may approve the batch, release it without changes, or move individual items out to the decision group. Before confirming, the user may ask to expand any batch item (full analysis plus on-the-spot re-check); expanded items move to the decision group and are decided individually.

**Batch group presentation note:** When batch grouping is active, add: "This grouping is a classification suggestion only — nothing is applied until you confirm. Each item shows its judgment basis; ask to expand any item if in doubt — expanded items move to the decision group."
==ATOM_END:batch_findings_mechanism==

After classification, present the findings (§Summary format) and wait for the user's decision per HARD RULE 3a:
- **Batch group:** one decision on the whole group per §Batch classification.
- **Decision group:** present each finding with its resolution type (fix_required / blocked) and wait for the user's decision per HARD RULE 3a. Do not offer a structured resolution menu.

### Summary format

```
────────────────────────────────────────────────────
Mode: scoped | full
Validate result: FAIL
Failed checks: N / Total findings: M
Findings:
  Batch group (N items) — fix fully determined by an objective standard:
    - {item}: {one-line fix} (based on: {standard reference})
    ...
  Decision group (M items) — need confirmation:
    1. {check-name}: FAIL — {reason} (fix_required | blocked)
    ...
────────────────────────────────────────────────────
```

`Total findings` equals the sum of batch group items and decision group items.

When no finding qualifies for the batch group, present flat:

```
Failed checks: N / Total findings: M
Findings:
  1. {check-name}: FAIL — {reason} (fix_required | blocked)
  ...
```

### Validate-specific notes

- **Re-validation rule:** After any fix is applied, the agent must NOT re-run validate automatically. Executing quality-gate commands is user-triggered only (see HARD RULE 2 in `framework/concepts.md`). The agent proposes a scoped re-check with the concrete command and waits for the user to trigger it. Affected-check mapping: acceptance item edits (any field) affect the Check 5 family — suggest `validate@{unit}:check-5`, since every sub-check reads item fields; `affects.*` edits additionally affect Check 6 — suggest `validate@{unit}:check-5` plus `validate@{unit}:check-6`; edits to spec body prose or appendices affect 5a/5b — suggest `validate@{unit}:check-5`. Example suggestion: "Fixes applied. Suggest re-running `validate@{unit}:check-{n}` to confirm. Shall I run it?" Until a re-run is triggered, do NOT write a pass cache and do NOT claim the fix is verified — report "fixed, pending re-confirmation". When a re-run is triggered by the user, the final validate result is based on the re-run, not on the pre-fix snapshot. Findings from the pre-fix snapshot that are no longer reproducible on the re-run are dropped, not carried forward as still-open. When a re-run changes an earlier finding, inform the user: "Re-validated affected checks after the fix. [finding] no longer holds. Remaining findings: ..."
- **Blocked items** (resolution_type: blocked) require user input — skip to next finding without suggesting a fix.

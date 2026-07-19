# Verify Checklist

## Overview

When an agent executes `spec_verify {unit}`, it uses the 7 steps defined in this file (6 analysis + 1 confidence assessment). This file is referenced by `framework/concepts.md` §3 — the agent reads this file at verify time, not proactively.

## Mode Selection

Before executing, read `framework/verification_scope.md` to determine the scope mode from the trigger phrase. The mode determines what to verify:

| Trigger | Mode | What to execute |
|---------|------|-----------------|
| `spec_verify {unit}` | scoped (default) | Git-aware: `git diff HEAD` → match changed files to spec content → verify that content (all 7 steps). See `framework/verification_scope.md` §Scoped Verify for detailed logic. |
| `spec_verify {unit}:{keyword}` | scoped | Match keyword to spec content by title, feature name, or structure → verify that content |
| `spec_verify {unit}:full` | full | Verify all spec content (all 7 steps, batch by spec structure) + cross-check |

**Output:** prefix with `Mode: scoped` or `Mode: full` and describe what was checked in natural language.

**Cache:** see `framework/validation_cache.md` for format.

## Core Principle

Verify is a **static structural alignment check** — it compares what the spec describes against what the code implements, using static file inspection (reading file content, searching text by pattern, locating files by name pattern) and read-only repository history queries. It cannot run code, call APIs, or capture runtime output. The "evidence" in verify is **code references**: file paths, line numbers, and code snippets proving structural existence and consistency.

**Adversarial stance:** verification starts from the assumption that the spec is NOT aligned with the code. Every ALIGNED claim must cite a deterministic check (grep, file existence, line count) — "I read the code and it looks correct" is not sufficient evidence for ALIGNED. Claims without deterministic evidence must be reported as CANNOT_DETERMINE.

After completing all analysis steps, the agent must report coverage confidence (see §Output Format Coverage).

## Target Selection

- If a candidate spec exists → verify code against **candidate** (the current working proposal). Mismatches trigger divergence resolution.
- If no candidate exists but a stable spec does → verify code against **stable** (check if current implementation still conforms to recorded truth). Do not enter divergence resolution — instead, recommend a `unit_fork`.

## Execution Rules

- **Subagent permissions:** may inspect file content, search text by pattern, locate files by name pattern, and query read-only repository history (e.g., `git log` for file timestamps). Must NOT modify files or execute commands that change state. In **scoped mode**, must NOT delegate to other agents. In **full mode**, the main agent may delegate batches to read-only sub-agents — each sub-agent follows the same permissions (read-only, no delegation chain).
- Each verifiable claim in the spec reports **ALIGNED** / **MISMATCH** / **CANNOT_DETERMINE** with code references.
- Evidence is always code-level (file:line, struct/function signatures, grep results) — never runtime output.
- For CANNOT_DETERMINE claims (e.g., pass_condition requires runtime verification): record the gap and continue.

## Output Format

```
Verify result: ALIGNED | MISMATCH
Target: candidate | stable
Items:
  - {item.id}: ALIGNED | MISMATCH | CANNOT_DETERMINE — code references
    Direction: (only if MISMATCH) spec_gap | code_gap | needs_design
    Resolution: update_candidate | implement_code | redesign | blocked
Scope: ALIGNED | MISMATCH — findings
Integrity: PASS | FAIL — findings
Coverage:
  - items_with_deterministic_evidence: N/M
  - items_reading_only: N
Divergence summary: (only if any MISMATCH)
  - {item.id}: spec_gap | code_gap | needs_design — description
    User verdict: ...
    Next step: ...
Summary: ...
```

---

## Step 1 — Structural alignment

**Purpose:** Cross-reference every static structure declared in the spec body against the actual implementation code. This catches missing interfaces, wrong signatures, and inconsistent data definitions that acceptance items alone might miss.

**Why this exists:** Acceptance items describe "what to verify" but the spec body describes "how it's built." If a protocol definition or data structure in the body differs from what's implemented, verify must catch it — even if the acceptance items pass.

**Execution steps:**

1. Read the full spec body (main flow, protocols, data contracts, error handling, state machines, appendices)
2. Extract all statically verifiable structural declarations:

| Declaration type | What to extract | How to verify in code |
|---|---|---|
| API endpoints / handlers | path, method, request/response types | Search for route registration, check handler function signature |
| Function signatures | name, parameters, return types | Search for function definition, verify signature |
| Data structures | struct/type definitions, field names, types, tags | Search for type definition, check fields and types |
| Enums / constants | allowed values, string representations | Search for const/enum block, verify values exist |
| Error codes / types | error type names, error messages | Search for error definition, check usage in error paths |
| State machines | state values, transition triggers | Search for state type, switch/case blocks |
| Configuration keys | key names, default values | Search for config struct or usage |

3. For each declaration found in the spec, locate the corresponding implementation:

```
IF declaration exists in spec AND matching implementation found:
  - Verify structural consistency (signatures match, field names match, types match)
  - Report ALIGNED with code reference (file:line)
IF declaration exists in spec BUT no matching implementation found:
  - Report code_gap — spec describes something code doesn't have
IF declaration exists in code BUT not in spec:
  - Report spec_gap — code has something spec doesn't describe
```

**PASS (ALIGNED):** All spec body declarations have structurally consistent implementations

**FAIL (code_gap):** Spec describes structures not found in code → code is incomplete

**FAIL (spec_gap):** Code has structures not described in spec → spec is stale

**Check method:** Spec body × implementation code — bidirectional structural cross-reference

---

## Step 2 — Acceptance alignment

**Purpose:** For each acceptance item, verify that the code structurally satisfies the pass_condition using static analysis. This is not about running tests — it is about confirming the code contains the structures, functions, and patterns needed to satisfy the condition.

**Why this is achievable:** A pass_condition like "Returns HTTP 201 with id and created_at" can be verified by checking the handler code returns 201 status and includes those fields in the response struct. The subagent cannot verify the code *works correctly*, but it can verify the code *is structured correctly*.

**Execution steps:**

For each acceptance item in the target spec:

1. Read `implementation_surface`, `verification_surface`, and `affects.files` to locate the implementation
2. Parse the `pass_condition` and extract specific verifiable assertions:

| pass_condition type | What to check in code | Verifiable? |
|---|---|---|
| "Returns HTTP {status}" | Handler returns that status code | ✅ Yes — search for status code in handler |
| "Returns {field} in response" | Response struct has that field | ✅ Yes — check response type definition |
| "Calls {function} when {condition}" | Function call exists in correct branch | ✅ Yes — read conditional block |
| "Returns error when {condition}" | Error return in conditional path | ✅ Yes — read error handling path |
| "Rate limit is {n} req/min" | Rate limiter configured with that value | ✅ Yes — read rate limiter config |
| "{algorithm} produces correct result" | Function exists with algorithm | 🔶 Partial — structure exists, correctness not verifiable |
| "System handles {n} concurrent users" | N/A without load testing | ❌ No — CANNOT_DETERMINE |

3. For each verifiable assertion, locate the corresponding code and compare:
   - Does the handler exist at the expected path?
   - Does it have the right status code?
   - Does the response type include the expected fields?
   - Are the error types defined and used in error paths?

4. For each verifiable assertion, report the grep command or file
   check used as evidence. Every ALIGNED claim must include a specific
   deterministic check:

   ✅ ALIGNED (with deterministic evidence):
     grep -n "return 201" user.go
     → line 42: w.WriteHeader(http.StatusCreated)
     → handler returns 201 (confirmed by grep command)

   ❌ Insufficient — no grep command cited, reading only:
     MUST be reported as CANNOT_DETERMINE

5. If an assertion cannot be verified with a deterministic command
   (e.g., "error message is user-friendly"), report as CANNOT_DETERMINE.

   {item.id}: CANNOT_DETERMINE
     - pass_condition: "error message is user-friendly"
     - Reason: requires human judgment, no deterministic grep available

Per-item report format:

```
{item.id}: ALIGNED
  - evidence: grep -n "201" src/api/user.go → line 42
  - deterministic: true

{item.id}: MISMATCH
  - Spec pass_condition: "returns 201"
  - Implementation: handler at src/api/user.go:42 returns 200
  - Direction: code_gap

{item.id}: CANNOT_DETERMINE
  - pass_condition: "handles 1000 concurrent requests"
  - Reason: requires load testing, not statically verifiable
```

**PASS (ALIGNED):** All items have structurally consistent implementations

**FAIL (MISMATCH):** One or more items have structural inconsistencies

**CANNOT_DETERMINE:** Items that require runtime verification — note them for human review

**Check method:** Acceptance item pass_condition × implementation code — assertion-level structural cross-reference

**Test design sub-check (for verification_type: testable items only):**

For each acceptance item with `verification_type: testable`, apply the `framework/test_decomposition_standard.md` standard as a baseline to identify significantly implied but missing test scenarios:

1. **Step 1 (happy path):** Does a test exist that exercises the primary success scenario? If the pass_condition describes a success outcome and no test covers it → MISMATCH (possible untested core behavior)
2. **Steps 2-4 (input variants, business rules, dependency failure):** Does the description or pass_condition strongly imply a scenario (e.g. "register" implies "email already exists", "create order" implies "invalid product ID") that has no corresponding test? If a reasonable developer would expect a test for that scenario and none exists → MISMATCH

This sub-check is **not** an exhaustive coverage audit. It flags obvious omissions. A single acceptance item may produce zero, one, or several test scenarios depending on its content. If the implementation is in a language or framework where tests are not written in the expected location, report CANNOT_DETERMINE instead of MISMATCH.

---

## Step 3 — Scope accuracy

**Purpose:** Cross-reference each acceptance item's `affects` declarations against the actual implementation. This catches undeclared scope and missing declarations.

**Execution steps:**

1. For each acceptance item with `affects.files`:
```
- Read each declared file
- Does the file contain implementation relevant to the pass_condition?
- Is the nature of the change consistent with the behavior described?
  (If item says "add login handler" but the file only has imports → flag)
```

2. For each acceptance item with `affects.rules`:
```
- Search for each declared rule name or pattern in the implementation files
- Is the rule being used? How?
- If the rule is not found in the implementation → flag (possible undeclared scope)
```

3. For each acceptance item with `affects.dependencies`:
```
- Search for each declared dependency in the implementation
- Is the dependency used? If not → flag
```

4. Cross-reference: compare declared `affects.files` against the set of files that contain actual implementation related to this acceptance item:
```
- Files with relevant code but not in affects.files → flag (under-declared scope)
- Files in affects.files but with no relevant code → flag (over-declared scope)
```

**PASS:** All affects declarations are accurate and complete

**FAIL (scope MISMATCH):** Undeclared scope or inaccurate declarations found

**Check method:** affects.* declarations × actual implementation — triple cross-reference (files, rules, dependencies)

---

## Step 4 — Retirement verification

**Purpose:** For replacement candidates, verify old code is fully removed with no remaining references.

**Execution steps:**

1. Check if any acceptance item has `verification_type == inspectable` AND `evidence_requirements` includes `old_code_deleted` and `no_remaining_refs`. If not, skip this step.
2. If `replacement`, for each acceptance item with `verification_type: inspectable` and `evidence_requirements` containing `old_code_deleted` and `no_remaining_refs`:
```
- Identify the old code paths from the spec (or from the replacement nature of the change)
- Verify old code files/directories no longer exist
- Search for remaining references to:
    - Old function names
    - Old import paths
    - Old configuration keys
    - Old module/package names
```
3. Report findings with grep evidence (file:line for any remaining references, or "no references found").

**PASS:** Old code removed, no remaining references

**FAIL:** Old code or references still exist

**Check method:** Replacement scope × file existence × grep for remaining references

---

## Step 5 — Implementation integrity

**Purpose:** Detect code behaviors not declared in the spec (spec_gap) and assess the impact of `not_runnable_yet` items.

**Execution steps:**

1. **Undocumented behavior scan:**
```
- Read the implementation files listed in affects.files across all acceptance items
- Identify any functions, conditionals, or data flows that implement behavior not described in the spec body or acceptance items
- For each undocumented behavior:
    - Is it a supporting implementation detail (e.g., helper function, logging)?
    - Or is it a behavioral change not declared in the spec (e.g., add caching, new API endpoint, new error handling path)?
    - If behavioral → report as spec_gap
```

2. **not_runnable_yet assessment:**
```
- Count acceptance items with not_runnable_yet: yes
- If all items are not_runnable_yet → report: "All acceptance items marked not_runnable_yet — verify cannot confirm alignment"
- If some items are not_runnable_yet:
    - List them
    - For each item, verify not_runnable_yet_reason is present
    - If missing → flag as concern: "Item {id} has not_runnable_yet: yes but no not_runnable_yet_reason"
    - If reason is present, does it reference external evidence?
      (e.g. issue/PR link, dependent system documentation, pending integration entry point)
    - If not → flag as concern: "Item {id} reason is self-attested — no external evidence"
    - Does the remaining runnable set provide meaningful coverage?
    - If not → flag as concern
```

**PASS:** No undocumented behavioral changes detected; not_runnable_yet items have documented reasons

**FAIL (spec_gap):** Undocumented behavioral changes found in code

**Quality concern:** All or most items are not_runnable_yet; runnable coverage is insufficient

**Quality concern (reasoning):** One or more not_runnable_yet items are missing not_runnable_yet_reason or lack external evidence

**Check method:** Implementation code × spec body — reverse cross-reference (code-to-spec direction)

---

## Step 6 — Stub & Placeholder Scan

**Purpose:** Systematically scan implementation files for known "not done" patterns. This is a deterministic check — running it twice produces identical results. It catches incomplete implementations that pass structural checks (the structure exists) but are placeholders.

**Execution steps:**

1. Collect all implementation files referenced in `affects.files` across all acceptance items. If `affects.files` is incomplete, also collect files from the spec body's implementation references.

2. For each file, run these commands and record findings:

   a. Stub/empty patterns:
   ```bash
   grep -n "return null\|return \[\]\|\bplaceholder\b\|not.*implement" <file>
   ```

   b. Debt markers (TODO, FIXME, XXX):
   ```bash
   grep -n "TODO\|FIXME\|XXX\|HACK" <file>
   ```

   c. Empty handler bodies:
   ```bash
   grep -n "return Response.json({})\|w.WriteHeader(204)" <file>
   ```

3. Per-file result:
   ```
   {file}: CLEAN | STUB_FOUND
     - line 5: // TODO: connect to database (debt_marker)
     - line 12: return Response.json({}) (empty_response)
   ```

4. Any stub finding is a MISMATCH. Direction is **code_gap** — code has placeholder where real implementation is expected.

**PASS:** No stubs or placeholders found

**FAIL (STUB_FOUND):** One or more files contain stub patterns

**Check method:** grep — deterministic, outputs are identical across runs

---

## Step 7 — Divergence resolution and stable-only mode

### Divergence resolution

When any item reports MISMATCH, classify each mismatch into one of four directions. **Present findings to the user — do not decide automatically. However, the agent MUST provide a reasoned suggestion based on the signal layer and review layer below.**

| Classification | Meaning | Next step |
|---|---|---|
| **spec_gap** | Code has behavior the spec doesn't describe. The candidate spec is stale. | Update candidate spec → re-run validate → re-run verify |
| **code_gap** | Spec describes behavior the code doesn't satisfy. Code is incomplete. | Implement code → re-run verify |
| **needs_design** | Neither spec nor code matches a coherent design. Needs rethinking. | Redesign candidate → validate → verify |
| **blocked** | Mismatch depends on external input or unresolved decisions. | User unblocks → re-run verify |

**Execution:**

For each MISMATCH item:
1. **Classify** — determine the direction (spec_gap / code_gap / needs_design / blocked)
2. **Signal layer** — collect metadata evidence (see below)
3. **Review layer** — read spec and code content to analyze design intent (see below)
4. **Present** — show mismatch evidence, signal layer, review layer, and agent's suggested direction
5. **Record** — the user's verdict and the agreed next step

---

#### Signal Layer

Collect metadata evidence to inform the suggestion. Each evidence item has a weight; the agent sums weighted evidence on both sides to determine confidence.

| Evidence | Favors fixing | How to get | Weight |
|----------|--------------|-----------|--------|
| Spec is **stable** (approved) | Code (spec is contract) | Check spec file path contains `stable/` | Strong |
| Spec is **candidate** (WIP) | Spec (code is evolving) | Check spec file path contains `candidate/` | Strong |
| Validate cache is fresh | Code (spec was recently checked) | Check `validate_result.md` timestamp vs file hashes | Strong |
| Code has tests for the behavior | Spec (behavior is intentional) | Search test files for related assertions | Strong |
| Spec modified after code | Code (spec reflects latest thinking) | Query version control for the file's last modification timestamp | Moderate |
| Code modified after spec | Spec (code evolved past spec) | Query version control for the file's last modification timestamp | Moderate |
| Code behavior referenced by other modules | Spec (other code depends on it) | Search for function calls / type usage | Weak |

**Confidence:**
```
fix_code_score = strong_code_fix * 2 + moderate_code_fix + weak_code_fix
fix_spec_score = strong_spec_fix * 2 + moderate_spec_fix + weak_spec_fix
delta = |fix_code_score - fix_spec_score|

confidence = high   if delta >= 3
confidence = medium if delta >= 1
confidence = low    otherwise (conflicting signals)
```

**If version control is not available:** skip timestamp evidence. Confidence capped at medium.

**If no test files found:** skip test evidence. Do not infer "no tests" as evidence for anything.

---

#### Review Layer

For each MISMATCH, the agent MUST read beyond the item itself to understand design intent. This is content-level analysis, not metadata counting.

**Scope — read:**

1. **Spec context** — the section/group containing the acceptance item, including sibling items, shared definitions (error codes, types, enums, data models), and rationale in section headers. If the item spans sections, read the entire spec file.

2. **Code context** — the implementation function body where the mismatch occurs, surrounding logic, related helper functions, and callers. Read enough to understand why the code behaves the way it does.

3. **Tests** — if test files exist for the relevant code module, read their assertions for the mismatched behavior. Test assertions are strong signals of intent.

**Analysis protocol:**

1. Extract intent from spec — what is the spec trying to achieve? Is the requirement internally consistent with sibling items?
2. Extract intent from code — why does the code behave differently? Is there a pattern or principle behind the code's approach?
3. Reconcile — which version better serves the overall design? Is one version clearly wrong (bug, typo, oversight)?
4. Edge cases — does one version handle edge cases that the other misses?

**Output:** reasoned judgment with confidence level.

**When to skip the review layer:**

| Condition | Action |
|-----------|--------|
| Structural-only mismatch (signature, type name) | Signal layer sufficient |
| Trivial mismatch (typo, whitespace) | Signal layer sufficient |
| CANNOT_DETERMINE | Review layer required (signal has no data) |
| Conflicting signals (low confidence) | Review layer recommended |

---

#### Presentation format

For each MISMATCH item, present in the following structure:

```
───────────────────────────────────────────────────
Item: {id}
Mismatch: spec says {X}, code does {Y}
───────────────────────────────────────────────────

[REVIEW]
  ├─ Spec context: {what spec section says, design intent}
  ├─ Code context: {how code handles it, why}
  ├─ Analysis: {reasoning chain}
  └─ Judgment: {direction} (confidence: high/medium/low)

[SIGNAL] (condensed)
  ├─ Spec: {stable/candidate} — favors fix {code/spec}
  ├─ Timestamps: spec {date}, code {date} — favors fix {code/spec}
  └─ Tests: {yes/no} — favors fix {code/spec}

Suggested direction: {spec_gap | code_gap | needs_design}
  {1-sentence rationale from review layer}
───────────────────────────────────────────────────
Do you agree, or do you see it differently?
(agree / disagree → specify your direction)
───────────────────────────────────────────────────
```

The signal layer is always shown in condensed form (3-4 lines). The review layer is the primary reasoning. If signal and review conflict, highlight the conflict explicitly.

### Stable-only mode

When no candidate spec exists (verify against stable):

```
1. Run Steps 1-6 against stable spec
2. If all ALIGNED → report ALIGNED. No further action.
3. If any MISMATCH:
   - The implementation has drifted from recorded stable truth
   - Do NOT enter divergence resolution
   - Report the drift and recommend unit_fork:
     "Current implementation diverges from stable spec at {details}.
     Recommend creating a candidate round (unit_fork) to reconcile the difference."
```

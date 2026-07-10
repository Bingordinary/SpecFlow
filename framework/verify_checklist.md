# Verify Checklist

## Overview

When an agent executes `spec_verify {unit}`, it uses the 6 steps defined in this file. This file is referenced by `framework/concepts.md` §3 — the agent reads this file at verify time, not proactively.

## Core Principle

Verify is a **static structural alignment check** — it compares what the spec describes against what the code implements, using only Read/Grep/Glob. It cannot run code, call APIs, or capture runtime output. The "evidence" in verify is **code references**: file paths, line numbers, and code snippets proving structural existence and consistency.

## Target Selection

- If a candidate spec exists → verify code against **candidate** (the current working proposal). Mismatches trigger divergence resolution.
- If no candidate exists but a stable spec does → verify code against **stable** (check if current implementation still conforms to recorded truth). Do not enter divergence resolution — instead, recommend a `unit_fork`.

## Execution Rules

- **Subagent permissions:** ALLOWED: Read, Grep, Glob. FORBIDDEN: Write, Edit, Bash, Task.
- Each acceptance item reports **ALIGNED** / **MISMATCH** / **CANNOT_DETERMINE** with code references.
- Evidence is always code-level (file:line, struct/function signatures, grep results) — never runtime output.
- For CANNOT_DETERMINE items (pass_condition requires runtime verification): record the gap and continue.

## Output Format

```
Verify result: ALIGNED | MISMATCH
Target: candidate | stable
Items:
  - {item.id}: ALIGNED | MISMATCH | CANNOT_DETERMINE — code references
    Direction: (only if MISMATCH) code_ahead | spec_ahead | needs_design
    Resolution: update_candidate | implement_code | redesign | blocked
Scope: ALIGNED | MISMATCH — findings
Integrity: PASS | FAIL — findings
Divergence summary: (only if any MISMATCH)
  - {item.id}: code_ahead | spec_ahead | needs_design — description
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
| API endpoints / handlers | path, method, request/response types | Grep for route registration, check handler function signature |
| Function signatures | name, parameters, return types | Grep for function definition, verify signature |
| Data structures | struct/type definitions, field names, types, tags | Grep for type definition, check fields and types |
| Enums / constants | allowed values, string representations | Grep for const/enum block, verify values exist |
| Error codes / types | error type names, error messages | Grep for error definition, check usage in error paths |
| State machines | state values, transition triggers | Grep for state type, switch/case blocks |
| Configuration keys | key names, default values | Grep for config struct or usage |

3. For each declaration found in the spec, locate the corresponding implementation:

```
IF declaration exists in spec AND matching implementation found:
  - Verify structural consistency (signatures match, field names match, types match)
  - Report ALIGNED with code reference (file:line)
IF declaration exists in spec BUT no matching implementation found:
  - Report spec_ahead — spec describes something code doesn't have
IF declaration exists in code BUT not in spec:
  - Report code_ahead — code has something spec doesn't describe
```

**PASS (ALIGNED):** All spec body declarations have structurally consistent implementations

**FAIL (spec_ahead):** Spec describes structures not found in code → code is incomplete

**FAIL (code_ahead):** Code has structures not described in spec → spec is stale

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
| "Returns HTTP {status}" | Handler returns that status code | ✅ Yes — grep for status code in handler |
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

4. Report per item with code-level evidence (file:line, code snippet):

```
{item.id}: ALIGNED
  - Handler CreateUser at src/api/user.go:42 returns 201
  - Response struct UserResponse at src/api/user.go:15 includes field `id` and `created_at`
  - Error path at src/api/user.go:55 handles ErrDuplicateEmail

{item.id}: MISMATCH
  - Spec pass_condition: "returns 201"
  - Implementation: handler at src/api/user.go:42 returns 200
  - Direction: spec_ahead

{item.id}: CANNOT_DETERMINE
  - pass_condition: "handles 1000 concurrent requests"
  - Reason: requires load testing, not statically verifiable
```

**PASS (ALIGNED):** All items have structurally consistent implementations

**FAIL (MISMATCH):** One or more items have structural inconsistencies

**CANNOT_DETERMINE:** Items that require runtime verification — note them for human review

**Check method:** Acceptance item pass_condition × implementation code — assertion-level structural cross-reference

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
- Grep for each declared rule name or pattern in the implementation files
- Is the rule being used? How?
- If the rule is not found in the implementation → flag (possible undeclared scope)
```

3. For each acceptance item with `affects.dependencies`:
```
- Grep for each declared dependency in the implementation
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

1. Check if the candidate's `source_basis` is `replacement`. If not, skip this step.
2. If `replacement`, for each acceptance item with `verification_type: inspectable` and `evidence_requirements` containing `old_code_deleted` and `no_remaining_refs`:
```
- Identify the old code paths from the spec (or from the replacement nature of the change)
- Verify old code files/directories no longer exist
- Grep for remaining references to:
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

**Purpose:** Detect code behaviors not declared in the spec (code_ahead) and assess the impact of `not_runnable_yet` items.

**Execution steps:**

1. **Undocumented behavior scan:**
```
- Read the implementation files listed in affects.files across all acceptance items
- Identify any functions, conditionals, or data flows that implement behavior not described in the spec body or acceptance items
- For each undocumented behavior:
    - Is it a supporting implementation detail (e.g., helper function, logging)?
    - Or is it a behavioral change not declared in the spec (e.g., add caching, new API endpoint, new error handling path)?
    - If behavioral → report as code_ahead
```

2. **not_runnable_yet assessment:**
```
- Count acceptance items with not_runnable_yet: yes
- If all items are not_runnable_yet → report: "All acceptance items marked not_runnable_yet — verify cannot confirm alignment"
- If some items are not_runnable_yet:
    - List them
    - Does the remaining runnable set provide meaningful coverage?
    - If not → flag as concern
```

**PASS:** No undocumented behavioral changes detected; not_runnable_yet items are reasonable

**FAIL (code_ahead):** Undocumented behavioral changes found in code

**Quality concern:** All or most items are not_runnable_yet; runnable coverage is insufficient

**Check method:** Implementation code × spec body — reverse cross-reference (code-to-spec direction)

---

## Step 6 — Divergence resolution and stable-only mode

### Divergence resolution

When any item reports MISMATCH, classify each mismatch into one of four directions. **Present findings to the user — do not decide automatically.**

| Classification | Meaning | Next step |
|---|---|---|
| **code_ahead** | Code has behavior the spec doesn't describe. The candidate spec is stale. | Update candidate spec → re-run validate → re-run verify |
| **spec_ahead** | Spec describes behavior the code doesn't satisfy. Code is incomplete. | Implement code → re-run verify |
| **needs_design** | Neither spec nor code matches a coherent design. Needs rethinking. | Redesign candidate → validate → verify |
| **blocked** | Mismatch depends on external input or unresolved decisions. | User unblocks → re-run verify |

**Execution:**
```
For each MISMATCH item:
1. Report the specific structural discrepancy (file:line, code snippet, spec reference)
2. Analyze the root cause:
   - Does the spec describe something the code lacks? → spec_ahead
   - Does the code do something the spec doesn't describe? → code_ahead
   - Are both present but inconsistent (e.g., wrong parameter name)? → classify by intent
3. Present to user with the example pattern:
   "Item `{id}`: spec says X but code does Y. Is the spec outdated (code_ahead) or code incomplete (spec_ahead)?"
4. Record the user's verdict and the agreed next step
```

### Stable-only mode

When no candidate spec exists (verify against stable):

```
1. Run Steps 1-5 against stable spec
2. If all ALIGNED → report ALIGNED. No further action.
3. If any MISMATCH:
   - The implementation has drifted from recorded stable truth
   - Do NOT enter divergence resolution
   - Report the drift and recommend unit_fork:
     "Current implementation diverges from stable spec at {details}.
     Recommend creating a candidate round (unit_fork) to reconcile the difference."
```

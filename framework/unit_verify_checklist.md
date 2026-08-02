# Verify Checklist

## Overview

When an agent executes `verify@ {unit}`, it uses the 7 steps defined in this file (6 analysis + 1 confidence assessment). This file is referenced by `framework/concepts.md` §3 — the agent reads this file at verify time, not proactively.

## Prerequisite — Read all unit files

Before executing any verify step, the agent must read the complete unit spec:

1. Read the main spec: `docs/specs/units/candidate/unit_{unit}.md`
2. Glob all candidate appendix files: `docs/specs/units/candidate/appendix/unit_{unit}_*.md`
3. For each appendix, read its frontmatter. Skip files with `status: exempt`.
4. Read the content of every non-exempt appendix file.

The unit's complete spec is the union of the main spec and all non-exempt appendix files. All verify steps that follow operate on this union — appendix content (API contracts, data types, error codes, state machines) is part of the spec and must be verified against code.

## Mode Selection

| Trigger | Mode | What to execute |
|---------|------|-----------------|
| `verify@ {unit}` | scoped (default) | git diff HEAD → match changed files to spec content → verify that content (all 7 steps) |
| `verify@ {unit}:{keyword}` | scoped | Match keyword to spec content by title, feature name, or structure → verify that content |
| `verify@ {unit}:full` | full | Verify all spec content (all 7 steps, batch by spec structure) + cross-check |

**Scoped selection logic:** Run `git diff HEAD`. Identify spec content referencing changed files (`affects.files`, `implementation_surface`, body file paths). Verify identified content using all 7 steps. No match → report "changed files not referenced in spec" and suggest full run.

**Edge cases:** No git changes, scoped cache fresh → report still valid. No git changes, no cache → auto fallback to full. See `framework/verification_scope.md` §Scoped Verify and §Edge cases for full detail.

**Output:** Prefix with `Mode: scoped` or `Mode: full`. For scoped: append note "This is not a full verification. Run `verify@ {unit}:full` for complete verification."

**Cache:** see `framework/validation_cache.md` for format.

## Core Principle

Verify is a **static structural alignment check** — it compares what the spec describes against what the code implements, using static file inspection (reading file content, searching text by pattern, locating files by name pattern) and read-only repository history queries. It cannot run code, call APIs, or capture runtime output. The "evidence" in verify is **code references**: file paths, line numbers, and code snippets proving structural existence and consistency.

**Adversarial stance:** verification starts from the assumption that the spec is NOT aligned with the code. Every ALIGNED claim must cite a deterministic check (grep, file existence, line count) — "I read the code and it looks correct" is not sufficient evidence for ALIGNED. Claims without deterministic evidence must be reported as CANNOT_DETERMINE.

After completing all analysis steps, the agent must report coverage confidence (see §Output Format Coverage).

## Target Selection

- If a candidate spec exists → verify code against **candidate** (the current working proposal). Mismatches trigger first-principles divergence analysis (Step 7).
- If no candidate exists but a stable spec does → verify code against **stable** (check if current implementation still conforms to recorded truth). Do not enter divergence resolution — instead, recommend a `unit_fork`.

## Execution Rules

- **Subagent permissions:** may inspect file content, search text by pattern, locate files by name pattern, and query read-only repository history (e.g., `git log` for file timestamps). Must NOT modify files or execute commands that change state. In **scoped mode**, must NOT delegate to other agents. In **full mode**, the main agent may delegate batches to read-only sub-agents — each sub-agent follows the same permissions (read-only, no delegation chain).
- Each verifiable claim in the spec reports **ALIGNED** / **MISMATCH** / **CANNOT_DETERMINE** with code references.
- Evidence is always code-level (file:line, struct/function signatures, grep results) — never runtime output.
- For CANNOT_DETERMINE claims (e.g., pass_condition requires runtime verification): record the gap and continue.

## Output Format

```
Verify result: PASS | FAIL      # FAIL = P0/P1 findings; PASS may carry P2/P3 pending items (see Step 8)
Target: candidate | stable
Items:
  - {item.id}: ALIGNED | MISMATCH (severity: P0|P1|P2|P3) | CANNOT_DETERMINE — code references
Scope: PASS | FAIL — findings
Integrity: PASS | FAIL — findings
Coverage:
  - items_with_deterministic_evidence: N/M
  - items_reading_only: N
First-principles divergence analysis: (only if any MISMATCH)
  - {item.id}: {suggested direction} (severity: P0|P1|P2|P3) — {rationale}
    User verdict: ...
    Next step: ...
Summary:
  - blocking_mismatches: N  (P0/P1 count)
  - non_blocking_mismatches: N  (P2/P3 count)
```

---

## Step 1 — Structural alignment

**Purpose:** Cross-reference every static structure declared in the spec body and appendices against the actual implementation code. This catches missing interfaces, wrong signatures, and inconsistent data definitions that acceptance items alone might miss.

**Why this exists:** Acceptance items describe "what to verify" but the spec (body + appendices) describes "how it's built." If a protocol definition or data structure in the spec differs from what's implemented, verify must catch it — even if the acceptance items pass.

**Execution steps:**

1. Read the full spec body AND all non-exempt appendix files (main flow, protocols, data contracts, error handling, state machines, API definitions from appendices)
2. Extract all statically verifiable structural declarations from both main spec and appendix content:

| Declaration type | What to extract | How to verify in code | Semantic role | Match semantics | Severity if missing |
|---|---|---|---|---|---|
| API endpoints / handlers | path, method, request/response types | Search for route registration, check handler function signature | Contractual | Exact match | P0 |
| Function signatures | name, parameters, return types | Search for function definition, verify signature | Contractual | Exact match | P0 |
| Data structures | struct/type definitions, field names, types, tags | Search for type definition, check spec-declared fields exist in code | Descriptive | Subset match (spec ⊆ code) | P1 |
| Enums / constants | allowed values, string representations | Search for const/enum block, verify values exist | Contractual | Exact match | P0 |
| Error type names | error type identifiers, sentinel variables | Search for error definition, check usage in error paths | Contractual | Exact match | P0 |
| Error messages | user-facing message strings | Search for message constants/format strings | Descriptive | Subset match (spec ⊆ code) | P2 |
| State machines | state values, transition triggers | Search for state type, switch/case blocks | Contractual | Exact match | P0 |
| Configuration keys | key names, default values | Search for config struct or usage | Descriptive | Subset match (spec ⊆ code) | P2 |

The **Semantic role** column categorizes each declaration by its purpose in the spec:

- **Contractual**: The declaration describes behavioral constraints the system must obey. Code must match exactly; any deviation affects functional correctness.
- **Descriptive**: The declaration describes the minimum contract of the implementation. Code may provide more information without violating the contract.
- **Annotative**: The declaration is auxiliary documentation (e.g., scope, rationale); inaccuracy does not affect functional correctness.

The **Match semantics** column defines the comparison protocol between spec and code:

- **Exact match**: Everything the spec declares must exist in code in exactly identical form. Any deviation (missing field, different signature) → MISMATCH, using the severity from the row.
- **Subset match (spec ⊆ code)**: The spec describes the minimum contract for the declaration. Code must contain all spec-declared fields/values; extra code fields/values do not constitute a MISMATCH — record as a note. Match rule: spec ⊆ code.

3. For each declaration found in the spec, locate the corresponding implementation. Use the **Match semantics** from the extraction table above to determine the correct outcome:

```
IF declaration exists in spec AND matching implementation found:
  - Read the declaration's row in the extraction table:

    Exact match → Verify all spec-declared fields/signatures/values exist in code
                  AND are identical.
                  All match → ALIGNED.
                  Any mismatch → MISMATCH (severity from table row).

    Subset match → Verify all spec-declared fields/values exist in code.
                   All exist → ALIGNED.
                     (extra code fields do not constitute a MISMATCH —
                      record as a note)
                   Spec-declared fields missing from code → MISMATCH
                     (severity from table row).

IF declaration exists in spec BUT no matching implementation found:
  → MISMATCH (severity from table row)
    — spec describes something code doesn't have.

IF a complete declaration exists in code BUT has no correspondence
   in any spec declaration:
  → Do NOT report here. Defer to Step 5 (surplus detection).
  → Exception: Subset match declarations whose spec-declared fields are all
    present in code are ALIGNED (extra code fields are not surplus).
```

4. **Code surface reverse check** — after completing the spec→code search above, for each implementation file that was visited during steps 1-3, run an independent code→spec scan:

```
a. Extract all code-level structures from the file:
   - Types, structs, interfaces (struct field names and types)
   - Method receivers (which types have which methods)
   - Public function declarations
   - Public constants, variables, error sentinels

b. For each extracted structure, check against the full spec:
   - Does the spec body (protocols, data contracts, terminology, error codes) describe it?
   - Is it referenced in any acceptance item's description or pass_condition?
   - No spec correspondence → SURPLUS candidate
     (do not classify — defer to Step 7)

c. Reports:
   - Zero surplus → "Code surface fully covered — no surplus at the file level"
   - Surplus found → MISMATCH (type: surplus) with structure name, type, and file:line
```

**PASS (ALIGNED):** All spec body declarations have structurally consistent implementations

**FAIL (MISMATCH):** Structural differences found between spec and code — defer classification to Step 7

**Check method:** Spec body × implementation code — bidirectional structural cross-reference

---

## Step 2 — Acceptance alignment

**Purpose:** For each acceptance item, verify that the code structurally satisfies the pass_condition using static analysis. This is not about running tests — it is about confirming the code contains the structures, functions, and patterns needed to satisfy the condition. For appendix content, cross-reference technical claims (API contracts, data type definitions, configuration keys) against code even if no acceptance item directly covers them.

**Why this is achievable:** A pass_condition like "Returns HTTP 201 with id and created_at" can be verified by checking the handler code returns 201 status and includes those fields in the response struct. The subagent cannot verify the code *works correctly*, but it can verify the code *is structured correctly*.

**Execution steps:**

For each acceptance item in the target spec, AND for each technical claim in appendix files:

1. Read `implementation_surface`, `verification_surface`, and `affects.files` to locate the implementation
2. Parse the `pass_condition` and extract specific verifiable assertions:

| pass_condition type | What to check in code | Verifiable? |
|---|---|---|
| "Returns HTTP {status}" | Handler returns that status code | Yes — search for status code in handler |
| "Returns {field} in response" | Response struct has that field | Yes — check response type definition |
| "Calls {function} when {condition}" | Function call exists in correct branch | Yes — read conditional block |
| "Returns error when {condition}" | Error return in conditional path | Yes — read error handling path |
| "Rate limit is {n} req/min" | Rate limiter configured with that value | Yes — read rate limiter config |
| "{algorithm} produces correct result" | Function exists with algorithm | Partial — structure exists, correctness not verifiable |
| "System handles {n} concurrent users" | N/A without load testing | No — CANNOT_DETERMINE |

3. For each verifiable assertion, locate the corresponding code and compare:
   - Does the handler exist at the expected path?
   - Does it have the right status code?
   - Does the response type include the expected fields?
   - Are the error types defined and used in error paths?

4. For each verifiable assertion, report the grep command or file
   check used as evidence. Every ALIGNED claim must include a specific
   deterministic check:

   ALIGNED (with deterministic evidence):
     grep -n "return 201" user.go
     → line 42: w.WriteHeader(http.StatusCreated)
     → handler returns 201 (confirmed by grep command)

   Insufficient — no grep command cited, reading only:
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

{item.id}: CANNOT_DETERMINE
  - pass_condition: "handles 1000 concurrent requests"
  - Reason: requires load testing, not statically verifiable
```

**PASS (ALIGNED):** All items have structurally consistent implementations

**FAIL (MISMATCH):** One or more items have structural inconsistencies — defer classification to Step 7

**CANNOT_DETERMINE:** Items that require runtime verification — note them for human review

**Check method:** Acceptance item pass_condition × implementation code — assertion-level structural cross-reference

**Test design sub-check (for verification_type: testable items only):**

This sub-check has two parts: **A — coverage completeness** (tests exist for implied scenarios) and **B — test meaningfulness** (existing tests are genuine). The agent runs both parts and reports findings per acceptance item.

**Language-agnostic approach:** The agent reads test files and self-identifies the testing framework (mock libraries, assertion libraries, test runner conventions) rather than relying on a hardcoded language list. When a test framework or assertion style is unfamiliar, the agent reports CANNOT_DETERMINE rather than guessing.

---

### Part A — Coverage completeness

Identify significantly implied but missing test scenarios:

1. **Happy path:** Does a test exist that exercises the primary success scenario? If the pass_condition describes a success outcome and no test covers it → CONCERN (possible untested core behavior)
2. **Input variants, business rules, dependency failure:** Does the description or pass_condition strongly imply a scenario (e.g. "register" implies "email already exists", "create order" implies "invalid product ID") that has no corresponding test? If a reasonable developer would expect a test for that scenario and none exists → CONCERN

Reference: `framework/test_decomposition_standard.md` provides decomposition methodology for deeper scenario discovery but is not required reading for this sub-check.

Part A is **not** an exhaustive coverage audit. It flags obvious omissions. A single acceptance item may produce zero, one, or several test scenarios depending on its content. If the implementation is in a language or framework where tests are not written in the expected location, report CANNOT_DETERMINE.

---

### Part B — Test meaningfulness

Existing tests are not automatically meaningful. This part checks whether the tests that *do* exist are structured as genuine validation — or are ritual tests that pass regardless of implementation correctness.

**Execution steps:**

For each acceptance item with `verification_type: testable`, locate the corresponding test files. For each test function found, apply the following checks:

#### B1 — Mock density

**Method:** Identify all import paths in the test file. Separate mock/stub/fake imports (e.g. `testify/mock`, `gomock`, `jest.mock`, `sinon`, `unittest.mock`, `pytest-mock`) from regular imports. Calculate the ratio.

```
mock_ratio = mock_imports / total_imports
```

**Language-agnostic detection:** Agent reads imports and identifies mock/stub/fake libraries by name convention and usage context rather than a hardcoded list. Libraries whose primary purpose is creating test doubles are counted as mocks.

| Ratio | Report |
|-------|--------|
| < 80% | No concern |
| ≥ 80% | CONCERN — "Mock density is {n}%. Most dependencies are mocked; only orchestration is exercised." |
| 100% | CONCERN — "Every dependency is mocked. Test exercises wiring only, not real behavior." |

Mock density above 80% is a **signal**, not a verdict. The agent uses it in combination with other Part B checks. A well-written test with 100% mocks that verifies interaction patterns (e.g. "did the service call repository with the correct transformed data") is meaningful. A test with 100% mocks that calls a function and asserts a tautology is not.

#### B2 — Assertion authenticity

**Method:** For each test function, agent reads the full body and evaluates whether assertions genuinely verify the outcome implied by the test name and acceptance item.

Checklist per test function:
- Does the test have at least one assertion?
- Does the assertion target an actual output value (return value, state change, side effect) rather than a fixed/tautological expression?
- If the test name describes an error scenario ("returns error when email exists"), does at least one assertion check the error (type, message, presence)?

**Reports:**

```
{item.id}: CONCERN — Test "TestRegister_DuplicateEmail" describes a conflict scenario
  but contains no error assertion. The test calls the register function but only
  asserts NoError. The conflict logic is never verified.
```

**Signal usage:** A test missing a meaningful assertion for its stated purpose is a strong indicator of ritual testing. Even one such test per acceptance item warrants a CONCERN.

#### B3 — Tautological assertions

**Method:** Scan test functions for assertion patterns that always pass regardless of implementation state.

**Language-agnostic detection:** Agent reads the assertion call and its arguments, then evaluates whether the assertion could fail under any reasonable code change. The agent does not use hardcoded patterns — it reasons about each assertion:

- Is the asserted value an unconditional literal? (`assert.Equal(42, 42)` → always passes)
- Is the asserted value the test infrastructure itself? (`assert.NotNil(t, t)` where one `t` is `*testing.T` → never nil)
- Is the assertion checking a property that is guaranteed by the test setup rather than the implementation? (mock returns a fixed value, then the assertion checks that same fixed value without transformation)

**Reports:**

```
{item.id}: CONCERN — Tautological assertion in TestRegister_Success
  assert.Equal(42, 42) at line 23 — compares literal to literal, cannot fail

{item.id}: CONCERN — Tautological assertion in TestGetUser
  assert.NotNil(t) at line 45 — t is *testing.T, always non-nil in a running test
```

A single tautological assertion in a healthy test file may be accidental. Multiple tautological assertions across an acceptance item's tests → stronger signal of ritual testing.

#### B4 — All-happy-path detection

**Method:** After Part A has confirmed a happy path exists, count all test functions associated with the acceptance item. If every test exercises a success scenario and none exercises error/invalid/edge paths → CONCERN.

This is distinct from Part A's check: Part A checks whether a *specific implied scenario* is missing. B4 checks the *overall profile* of existing tests — a complete lack of negative testing.

```
{item.id}: CONCERN — All {N} tests are happy-path only. No test exercises
  validation rejection, business rule conflict, or dependency failure.
```

#### B5 — Mock-through detection

**Method:** For each test function, agent traces the data flow across three points:

1. **Mock setup:** What value does the mock return? (`mock.On("Create", ...).Return(User{ID: 1})`)
2. **Function call:** How is the mocked value consumed? (`result := svc.Register(...)`)
3. **Assertion:** What does the assertion check? (`assert.Equal(t, 1, result.ID)`)

If the mock return value passes through the function without transformation, validation, or conditional logic, and the assertion checks the same (or derived) value → the test exercises only the mock, not the implementation.

**Judgment criteria:**

Agent evaluates whether a realistic implementation defect would cause this test to FAIL:

```
Would this test fail if the function body were replaced with a no-op / passthrough?
  - Mock returns User{ID: 1}
  - Function: return mock.Create(...)  (direct passthrough — no logic)
  - Assertion: assert.Equal(t, 1, result.ID)
  → Test passes. No implementation logic is exercised. → CONCERN

Would this test fail if the validation logic were broken?
  - Mock returns nil error
  - Function: validates input, calls mock, transforms result
  - Assertion: assert.Equal(t, "formatted_name", result.Name)
  → Test fails if transformation breaks. Implementation logic is exercised. → No concern
```

**Reports:**

```
{item.id}: CONCERN — Test "TestRegister_Success" exercises mock passthrough only.
  Mock returns User{ID: 1, Name: "a"}, service returns it unchanged,
  assertion checks ID == 1. The test passes even if all business logic
  is removed.
```

#### B6 — Test naming signal

**Method:** Scan test function names for patterns that suggest lack of care:

- Numbered names: `Test1`, `Test2`, `test_1` (sequential numbering without semantic meaning)
- Generic handlers: `TestHandler`, `TestFunc`, `TestMethod`, `test_handler`
- Vague names: `TestSomething`, `TestMisc`, `test_stuff`

**This check is auxiliary only.** A single generically-named test is not a concern. But when B6 flags multiple tests AND other Part B checks (B1–B5) also signal concerns, the naming pattern strengthens the overall assessment.

```
{item.id}: CONCERN (low confidence) — Test functions "Test1", "Test2", "Test3"
  found. Naming is sequential with no semantic information.
```

---

### Part A/B integration

Part A and Part B findings are reported together per acceptance item:

```
{item.id}: ALIGNED
  - evidence: grep -n "201" src/api/user.go → line 42
  - deterministic: true
  Part A: No concerns
  Part B:
    - B1 Mock density: 60% — no concern
    - B4 All happy path: CONCERN — 3 tests, all success scenarios, no error path
```

Part B findings are recorded under the item in the verify output as CONCERN-level annotations. They do not change the item-level verdict (ALIGNED / MISMATCH / CANNOT_DETERMINE).

**Edge case:** If test files are not in a language the agent can parse with confidence, or if the test framework is unfamiliar, report CANNOT_DETERMINE for all Part B checks on that acceptance item. Do not guess.

**Edge case:** If acceptance item has no tests at all, Part A flags the missing scenarios; Part B is skipped entirely (no tests to evaluate).

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

**FAIL (scope MISMATCH):** Undeclared scope or inaccurate declarations found — defer classification to Step 7

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

**PASS:** Old code removed, no remaining references

**FAIL:** Old code or references still exist

**Check method:** Replacement scope × file existence × grep for remaining references

---

## Step 5 — Implementation integrity & surplus detection

**Purpose:** Discover code designs not declared in the spec (code surplus) and assess non-runnable items. Do not classify surplus findings yet — defer to Step 7.

**Execution steps:**

### Part A — Code surface surplus analysis

This analysis operates at the **design surface** level — package structure, core types, entry points, cross-cutting mechanisms — not individual function signatures. The goal is to systematically discover code designs that exist but are not recorded in the spec.

#### 1. Discover the code design surface

Execute each step below in order and record findings:

```
a. Read the directory structure
   - List all packages/modules under the implementation_surface paths
   - Infer each package's responsibility from package names, directory layout, and file organization

b. Read each package's public interface
   - What types/interfaces/structs does the package export?
   - What are the main entry points (constructors, factory functions, handler registrations)?
   - What are the core abstractions?

c. Read initialization/boot code
   - How are components wired together (main, init, bootstrap)?
   - What gets registered at startup (routes, middleware, subscribers, scheduled tasks)?
   - What cross-cutting mechanisms are initialized (cache pools, connection pools, rate limiters)?

d. Trace cross-file data flow
   - How do components communicate (direct call, events, message queue)?
   - Are there event buses, middleware chains, pipelines not mentioned in the spec?
   - Are there interception points (hooks, decorators, middleware) not described in the spec?

e. Identify cross-cutting mechanisms
   - Caching layer
   - Retry logic
   - Rate limiting
   - Logging / observability
   - Circuit breaker
   - Background / scheduled tasks
   - Feature flags
```

#### 2. Match against the spec surface (main spec + appendices)

```
For each design construct discovered in step 1:
- Does the spec body (protocol, architecture, responsibility sections) describe it?
- Does any appendix file describe it (API contracts, data types, component trees)?
- Does any acceptance item's description or pass_condition imply it?
- Is it referenced in the spec's terminology or data contracts?

If a construct has no correspondence in either the main spec or any appendix → record as surplus candidate.
```

#### 3. Filter: is it a real design decision or an implementation detail?

```
For each surplus candidate, determine:

Is a real design decision (must report):
- Has its own type/abstraction → yes
- Other code depends on it → yes (has consumers, not dead code)
- Has tests → strong signal it is intentional design
- Can be independently described as a responsibility/mechanism → yes
- Part of the public API surface (exported, cross-package boundary)

Is an implementation detail (ignore):
- Merely a utility/helper function
- No independent architectural significance
- Does not change the system's responsibility boundaries
- Private/internal type with no external consumers
- **Behavior is already covered by a Contractual declaration in the spec**
  (e.g., EventSender interface is an internal realization of the
  StateMachine already described in spec §3.3 — not surplus)
```

#### 4. Report

```
Zero surplus → "Code design surface fully covered by spec — no surplus"

Surplus found → report each as MISMATCH (type: surplus) with:
- Design description (what it is)
- Code location (file and line)
- Evidence (why judged as a real design decision)

Do not classify the resolution direction — defer to Step 7.
```

2. **Non-runnable assessment:**
```
- Count acceptance items with runnable: no
- If all items are runnable: no → report: "All acceptance items marked runnable: no — verify cannot confirm alignment"
- If some items are runnable: no:
    - List them
    - For each item, verify not_runnable_reason is present
    - If missing → flag as concern: "Item {id} has runnable: no but no not_runnable_reason"
    - If reason is present, does it reference external evidence?
      (e.g. issue/PR link, dependent system documentation, pending integration entry point)
    - If not → flag as concern: "Item {id} reason is self-attested — no external evidence"
    - Does the remaining runnable set provide meaningful coverage?
    - If not → flag as concern
```

**PASS:** No surplus design found; non-runnable items have documented reasons

**FAIL (MISMATCH):** Surplus design found — defer classification to Step 7

**Quality concern:** All or most items are non-runnable; runnable coverage is insufficient

**Quality concern (reasoning):** One or more non-runnable items are missing not_runnable_reason or lack external evidence

**Check method:** Implementation code design surface × spec body — reverse cross-reference (code-to-spec design direction)

---

## Step 6 — Stub & Placeholder Scan

**Purpose:** Systematically scan implementation files for known "not done" patterns. This is a deterministic check — running it twice produces identical results. It catches incomplete implementations that pass structural checks (the structure exists) but are placeholders.

**Execution steps:**

1. Collect all implementation files from `implementation_surface` paths and `affects.files` across all acceptance items. If `affects.files` is incomplete, also collect files from the spec body's implementation references.

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

4. Any stub finding is a MISMATCH — code has placeholder where real implementation is expected (do not classify yet — defer to Step 7)

**PASS:** No stubs or placeholders found

**FAIL (MISMATCH):** One or more files contain stub patterns — defer classification to Step 7

**Check method:** grep — deterministic, outputs are identical across runs

---

## Step 7 — First-principles divergence analysis

**Purpose:** When Steps 1-6 detect mismatches, determine the root cause and correct direction using first-principles reasoning — not presence-based heuristics. Each mismatch gets independent analysis via a dedicated sub-agent.

### How it works

For each MISMATCH item detected in Steps 1-6, launch a **read-only analysis sub-agent**. Each sub-agent analyzes one mismatch independently, without cross-contamination from other items.

> **Full mode note:** In full mode, Steps 1-6 are distributed across batching sub-agents. Those are detection-only — they do not classify. After all batching sub-agents return, the main agent launches one analysis sub-agent per mismatch for Step 7. Analysis sub-agents are separate from batching sub-agents.

### Sub-agent protocol

**Input provided by main agent:**

| Field | Description |
|-------|-------------|
| Item ID | Identifier from the spec |
| Mismatch type | structural / acceptance / scope / stub / surplus |
| Spec content | Exact spec text (section, pass_condition, or declaration) with file path and line |
| Code content | Exact code that differs, with file path and line |
| Context scope | Instructions on what to read beyond the mismatch point |

For surplus mismatches, the first-principles analysis additionally evaluates:
- Is this a genuine design decision that belongs in the spec? → **spec_gap**
- Does this conflict with the spec's declared architecture or boundaries? → **boundary_violation**
- Does this duplicate or replace something the spec already describes differently? → **coding_gap**

**Context scope — the sub-agent MUST read:**

| Context | What to read |
|---------|-------------|
| Spec context | The section/group containing the item, sibling items, shared definitions (error codes, types, enums, data models), rationale in section headers, and relevant appendix sections |
| Code context | The implementation function body where the mismatch occurs, surrounding logic, related helpers, and callers |
| Tests | If test files exist for the relevant code module, read their assertions for the mismatched behavior |
| Appendix context | If the mismatch involves an appendix claim, read the full appendix file to understand the claim's context and relationship to the main spec |

**Analysis instruction (first-principles framework):**

The sub-agent follows this reasoning chain. Each step must be answered explicitly before moving to the next:

```
1. What is the system supposed to do at this point?
   - Read spec context (section, sibling items, shared definitions, rationale)
   - Extract the design intent — what problem is this declaration trying to solve?

2. What does the code actually do?
   - Read code context (function body, surrounding logic, callers, related helpers)
   - Extract the code's intent — why does it behave this way?

3. Compare intents:
   - Do spec and code agree on the goal but differ on implementation?
   - Do they disagree on the goal itself?
   - Is one side clearly wrong (typo, dead code, outdated reference)?
   - Does one side handle edge cases the other misses?

4. Root cause analysis (choose the best fit):
   - Code is incomplete — spec intent is clear, code hasn't caught up
   - Spec is stale — code has evolved, spec wasn't updated
   - Design divergence — both sides made different valid trade-offs
   - Accident — bug, typo, copy-paste error
   - External dependency — blocked on something outside this unit

5. Recommend direction:
   - spec_gap: code's behavior is correct, spec needs updating
   - code_gap: spec's intent is correct, code needs updating
   - needs_design: neither side is clearly right — the design itself needs rethinking
   - blocked: the mismatch depends on an external input or unresolved decision

6. Confidence:
   - high: clear evidence supports one direction
   - medium: evidence leans one way but not definitive
   - low: conflicting signals, cannot determine with confidence
```

**Signal layer (metadata evidence):**

The sub-agent MAY also collect the following metadata to support its reasoning. Each evidence item has a weight — the sub-agent sums weighted evidence to determine confidence if needed:

| Evidence | Favors fixing | Weight |
|----------|--------------|--------|
| Spec is **stable** (approved) | Code (spec is contract) | Strong |
| Spec is **candidate** (WIP) | Spec (code is evolving) | Strong |
| Validate cache is fresh | Code (spec was recently checked) | Strong |
| Code has tests for the behavior | Spec (behavior is intentional) | Strong |
| Spec modified after code | Code (spec reflects latest thinking) | Moderate |
| Code modified after spec | Spec (code evolved past spec) | Moderate |
| Code behavior referenced by other modules | Spec (other code depends on it) | Weak |

If version control is available, query `git log` for file timestamps. If no test files exist, skip test evidence. Do not infer "no tests" as evidence for anything.

The signal layer is supplementary — it does not override first-principles reasoning. Its primary use is to provide confidence context when the content-level analysis alone cannot reach high confidence.

**Output format:**

```
Item: {id}
────────────────────────────────────
Spec intent: {what the spec is trying to achieve, with context}
Code intent: {what the code does and why}
Comparison: {goal agreement? implementation difference? edge case handling?}
Root cause: {incomplete | stale | divergence | accident | blocked}
Suggested direction: {spec_gap | code_gap | needs_design | blocked}
Files involved: {spec and/or code file paths the suggested fix would touch}
  - List every file the fix would modify; the main agent uses this for batch classification scope evaluation
Severity: {P0 | P1 | P2 | P3}
  - Derive from the declaration's table row in Step 1 (severity if missing).
  - For surplus findings: determine by the construct's semantic role —
    public API = P0/P1, internal implementation detail = P2.
  - For scope findings (Step 3): annotative role → default P2.
Confidence: {high | medium | low}
Rationale: {2-3 sentence first-principles reasoning chain}
Signal layer: {condensed metadata summary, if collected}
```

**Constraints:**

- Read-only: MUST NOT modify files
- Independent: each mismatch is analyzed separately, without reference to other mismatches
- If version control history is available, MAY query `git log` for the relevant files. If not available, skip timestamp evidence — confidence capped at medium.

### Batch classification (main agent)

When MISMATCH items exist, the main agent classifies each finding into a **batch group** or a **decision group** before presenting. Classification aggregates fields the Step 7 sub-agents already returned (root cause, suggested direction, confidence, severity, files involved) — it adds no new analysis; for batch group candidates it only runs lightweight assertion re-verification (see Assertion re-verification below).

**Activation threshold:** Batch grouping is enabled only when the total MISMATCH count ≥ 10 AND the batch group would contain ≥ 3 items. Below the threshold, present findings flat (§Summary format without grouping).

**Batch group eligibility (ALL must hold):**

A. **Direction is unambiguous:**
   - Root cause is `accident` or `incomplete` (exclude `divergence`, `blocked`, `stale`)
   - Suggested direction is a single value `code_gap` or `spec_gap` (exclude `needs_design` and any item whose direction is an "either/or" verdict)
   - Confidence is `high`
   - The correct behavior has an undisputed source: the spec states it explicitly (e.g., an error code table) or an existing correct pattern in the codebase (e.g., the same construct written correctly elsewhere). If confirming "what is correct" requires interpretation → exclude.

B. **Small change scope:**
   - Involves ≤ 3 files, all within the same change domain (all code, or all spec documents) — no cross-domain spec↔code linkage
   - Change type ∈ {fix dead branch / wrong parameter, wire an existing but unconnected call, spec wording correction, add missing test}
   - Exclude: new mechanisms, API / data-structure contract changes, items requiring core changes or exposing a core gap

C. **No user decision needed:**
   - The fix does not change currently-working behavior (it repairs a dead branch or a missing piece that never took effect)
   - No boundary verdict, no "shrink spec vs change code" either/or

Any item failing any criterion → decision group. **P0/P1 items always go to the decision group** — the batch group only contains P2/P3.

==ATOM_BEGIN:batch_findings_mechanism==
**Assertion re-verification:** After eligibility passes, the main agent re-verifies each batch group candidate's core assertion with 1-2 deterministic checks:
- Sub-agent claims "X is missing" → confirm X is really absent from the cited location (read the file, check existence, or grep as appropriate)
- Sub-agent claims "the cited source states X explicitly" → read the cited line, confirm X is really written there, without vague wording ("or", "suggested", alternatives)
- Sub-agent claims "the correct pattern exists elsewhere" → confirm that reference really exists

Re-verification failure → item moves to the decision group. This runs before execution because a post-execution check re-run cannot detect a wrong-direction fix — once the documents are made consistent, the re-run passes and the error is cemented.

**Execution boundary:** Classification is presentation only — it is not an authorization to act. Nothing is implemented until the user explicitly agrees to the whole batch group. The user may approve the batch, release it without changes, or move individual items out to the decision group. Before confirming, the user may ask to expand any batch item (full analysis plus on-the-spot re-check); expanded items move to the decision group and are decided individually.

**Batch group presentation note:** When batch grouping is active, add: "This grouping is a classification suggestion only — nothing is applied until you confirm. Each item shows its judgment basis; ask to expand any item if in doubt — expanded items move to the decision group."
==ATOM_END:batch_findings_mechanism==

### Analysis collection

After all sub-agents return:

1. Collect all analysis results
2. Classify findings per §Batch classification (skip when the activation threshold is not met)
3. Present the consolidated findings summary (§Summary format)
4. Wait for the user's decision per HARD RULE 3a:
   - **Batch group:** one decision on the whole group per §Batch classification.
   - **Decision group:** present each finding with its suggested direction and wait for the user's decision. Do not offer a structured resolution menu.

### Summary format

```
────────────────────────────────────────────────────
Mode: scoped | full
Verify result: PASS | FAIL   # P0/P1 → FAIL; all aligned or P2/P3 only → PASS (pending items in counts)
Target: candidate | stable
Blocking mismatches: N  (severity P0/P1)
Non-blocking mismatches: N  (severity P2/P3)
Findings:
  Batch group (N items) — direction resolved, suggested for batch handling:
    - {item.id}: {one-line direction} (P{n}, based on: spec@file:line | code@file:line)
    - ...
  Decision group (M items) — decided one by one:
    - {item.id}: spec says {X}, code does {Y}
      → {suggested direction} (severity: P{n}, confidence: {level})
    - ...
────────────────────────────────────────────────────
```

When batch grouping is inactive (threshold not met), the Findings section uses the flat format:

```
Findings:
  - {item.id}: spec says {X}, code does {Y}
    → {suggested direction} (severity: P{n}, confidence: {level})
  ...
```

**Batch group presentation note:** The note text is defined in §Batch classification.

### Direction table

| Direction | Meaning | Next step | Promote gate |
|-----------|---------|-----------|:---:|
| **spec_gap** | Code's behavior is correct, spec needs updating | Update candidate spec → suggest re-running validate (affected checks) and verify (affected content); user triggers | P0/P1 → BLOCK; P2/P3 → Allow |
| **code_gap** | Spec's intent is correct, code needs updating | Implement code → suggest re-running verify (affected content); user triggers | P0/P1 → BLOCK; P2/P3 → Allow |
| **needs_design** | Neither side matches a coherent design — needs rethinking | Redesign candidate → suggest validate then verify; user triggers | P0/P1 → BLOCK; P2/P3 → Allow |
| **blocked** | Mismatch depends on external input or unresolved decision | User unblocks → suggest re-running verify; user triggers | P0/P1 → BLOCK; P2/P3 → Allow |

### Re-check after fixes

Executing quality-gate commands is user-triggered only (see HARD RULE 2 in `framework/concepts.md`). After a fix is applied, the agent must NOT re-run verify automatically. The agent proposes a scoped re-check with the concrete command and waits for the user to trigger it:

- **Spec-only fixes** (spec prose, acceptance items, affects fields): scoped selection maps spec-file edits to affected content per `framework/verification_scope.md` §Scoped Verify step 3 (spec files have no code reference — the mapping is direct: edited body sections → those chapters and their corresponding acceptance items; edited item fields → those items; edited appendix files → the chapters referencing them). Propose `verify@{unit}` (scoped — it reports the mapping) or `verify@{unit}:{keyword}` for a targeted re-check; the user may choose full.
- **Code fixes**: propose `verify@{unit}` (scoped — matches the changed code files).
- Until a re-check is triggered, do NOT claim the fix is verified — report "fixed, pending re-confirmation". A scoped re-check writes a `mode: scoped` cache only when PASS (all items aligned, see Step 8); promote's mode gate rejects scoped caches (`framework/validation_cache.md`), so cache freshness is restored only by a user-triggered `verify@{unit}:full`.

After presenting the summary:
- If batch grouping is active: stop and wait for the user's decision on the batch group (approve / release / move items out) and on each decision-group finding. Nothing is implemented without explicit user agreement.
- If batch grouping is inactive: present each finding with its suggested direction and stop. The user decides the next step per HARD RULE 3a. Do not offer a structured resolution menu.

### Stable-only mode

When no candidate spec exists (verify against stable):

```
1. Run Steps 1-6 against stable spec
2. If all items are ALIGNED → report PASS. No further action.
3. If any MISMATCH:
   - The implementation has drifted from recorded stable truth
   - Do not suggest spec modifications (cannot modify stable spec directly)
   - Report the drift and recommend unit_fork:
     "Current implementation diverges from stable spec at {details}.
     Recommend creating a candidate round (unit_fork) to reconcile the difference."
```

---

## Step 8 — Write verify cache (main agent)

After all 7 steps complete, determine cache action based on the highest severity MISMATCH (the finding-level verdict). The suggested direction (spec_gap / code_gap / needs_design / blocked) does not change the gate — blocking is determined by severity only. The cache `result` field uses the gate vocabulary: `pass` means no P0/P1 blocking findings; `fail` is reserved for the P0/P1 state, which never writes a cache (see below).

- **If all ALIGNED:** write verify cache per `framework/validation_cache.md` format:
  - Create `docs/specs/meta/validation/unit/{name}/` directory if needed
  - Collect SHA-256 hashes of all files read during verification
  - Write `verify_result.md` with `result: pass`, severity counts at 0, `target: candidate|stable`, `mode: scoped|full`, `blocking: false`, file hashes

- **If any P0/P1 MISMATCH exists (verify FAIL):** delete existing `verify_result.md` if present. Do not write cache — `result: fail` never appears in a verify cache. Report blocking findings. Agent must stop and not proceed to promote.

- **If all mismatches are P2/P3 (non-blocking, verify PASS):**
  - **Full run (`:full`):** write verify cache with `blocking: false`:
    - Collect SHA-256 hashes of all files read during verification
    - Write `verify_result.md` with `result: pass`, `blocking: false`, severity counts (`p0_count`...`p3_count`), `target: candidate|stable`, `mode: full`, file hashes
    - Report findings as non-blocking — agent may continue (P2/P3 findings do not block promote).
  - **Scoped run:** report findings only — do NOT write a cache. A scoped cache cannot pass promote's mode gate, and writing would downgrade an existing full cache. See `framework/validation_cache.md`.

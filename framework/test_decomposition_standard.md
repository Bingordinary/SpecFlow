# Test Decomposition Standard

## Scope

This standard applies to acceptance items with `verification_type: testable`.

It defines how to derive a set of unit test scenarios from a single acceptance item's `description` and `pass_condition`. The result is a **test scenario checklist** — an ordered list of what scenarios should be tested and what each verifies.

This standard is **language-agnostic**. It describes what to test, not how to write the test file structure.

When `description` is already written in Gherkin-style Given/When/Then format, the four decomposition steps below serve as a completeness check rather than a generation process. Each scenario in the description maps to one or more test cases; the steps verify no category is missing.

## Decomposition Steps

For each acceptance item, apply the following four steps in order.

### Step 1 — Happy path

Extract the normal success case from the acceptance item.

- **Input**: `description` + `pass_condition`
- **Method**: Identify the primary success scenario. What input produces the stated success output?
- **Output**: One happy path test
- **Common sign it is missing**: The acceptance item only exists conceptually but no test asserts the success behavior works.

### Step 2 — Invalid input variants

For every input parameter visible in the `pass_condition` or `description`, identify invalid values.

- **Input**: All parameters mentioned or implied by the acceptance item
- **Method**:

| Input type | Common invalid variants |
|---|---|
| String | empty string, exceeds max length, illegal characters, wrong format (e.g. non-email for email field) |
| Numeric | zero, negative, exceeds max value, floating point where integer expected |
| Enum / choice | value not in allowed set, null |
| Optional field | missing entirely |
| Collection | empty list, exceeds max size, duplicate entries |
| Object/body | malformed structure, missing required nested fields |

- **Output**: One test per distinct invalid variant that is realistically guardable
- **Rule of thumb**: If writing an `if` check for it in production code, write a test for it.
- **Common sign it is missing**: The code has input validation but no test exercises the rejection path.

### Step 3 — Business rule conflicts

Identify what conditions cause the operation to be rejected by business logic (not input format).

- **Input**: `description` + `pass_condition` + domain knowledge
- **Method**: Ask what preconditions could fail:

| Scenario type | Example |
|---|---|
| Resource already exists | Register with an email that is already taken |
| Resource not found | Delete a non-existent ID |
| State conflict | Cancel an already-cancelled order |
| Permission denied | Non-admin tries to modify admin settings |
| Quota exceeded | User has reached max number of allowed items |
| Rate limited | Too many requests in a time window |

- **Output**: One test per realistic business rule conflict
- **Common sign it is missing**: The acceptance item describes a constrained operation but only the happy path is tested.

### Step 4 — External dependency failure

Identify how the acceptance item should behave when each external dependency is unavailable or returns an error.

- **Input**: Affected dependencies from `affects.dependencies` or implied by `description`
- **Method**: For each dependency, consider:

| Dependency | Failure mode |
|---|---|
| Database | Connection timeout, query error, uniqueness constraint violation |
| Cache | Miss, connection refused, stale data |
| External API | Timeout, 5xx, unexpected response format |
| File system | Permission denied, disk full |
| Message queue | Publish timeout, broker unavailable |

- **Output**: One test per dependency failure mode that the code is expected to handle
- **Exception**: If `verification_surface` is not `internal_flow` (e.g. `api`) and the dependency cannot be simulated at the test level, mark as `covered_by: integration` and skip the unit test requirement.
- **Common sign it is missing**: The acceptance item depends on an external system but no test covers "what happens when that system is down."

## Usage

### Agent writing code

For each `verification_type: testable` acceptance item, apply Steps 1-4 as a mental checklist before writing tests. Not every step always produces test cases (e.g. an acceptance item with no external dependencies produces nothing for Step 4). The output is a list of test scenarios to implement.

This is **guidance, not a hard rule** — skip steps when they do not apply.

If the acceptance item's `description` already contains Gherkin-style scenarios, translate each Given/When/Then block directly into a test case. Then apply Steps 1-4 to verify coverage — if a step identifies a scenario type not present in the description, add it.

### Verify checking test design

The verify step evaluates both **coverage completeness** (are tests missing?) and **test meaningfulness** (are existing tests genuine?). Both are defined in `framework/unit_verify_checklist.md` Step 2's Test Design Sub-check (Parts A and B).

#### Part A — Coverage completeness (missing test detection)

For each `verification_type: testable` acceptance item, the verify step uses this standard as the baseline to identify **significantly implied but missing** test scenarios:

1. Does a test exist for Step 1 (happy path)? → if not, CONCERN
2. For Steps 2-4: does the `description` or `pass_condition` strongly imply a scenario that has no corresponding test? → if yes, CONCERN

Verify Part A does not check for exhaustive coverage of all possible inputs. It flags **obvious omissions** — scenarios that a reasonable developer would expect to exist given the acceptance item's content.

#### Part B — Test meaningfulness (existing test quality)

Existing tests may pass but lack genuine validation value. The verify step checks for ritual testing patterns:

- **Mock density:** Are nearly all dependencies mocked (orchestration-only testing)?
- **Assertion authenticity:** Do assertions actually verify the claimed behavior?
- **Tautological assertions:** Are there assertions that always pass regardless of implementation?
- **All-happy-path bias:** Do all existing tests only exercise success scenarios?
- **Mock-through detection:** Does a test pass mock data through and assert it unchanged (exercising only the mock, not the implementation)?
- **Naming signal:** Are test names semantically empty (auxiliary indicator)?

These checks are language-agnostic. The verify agent self-identifies testing frameworks and assertion patterns from the test file content. See `framework/unit_verify_checklist.md` Step 2 Part B for full methodology.

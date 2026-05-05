---
rule_id: g_rule_repository_baseline
rule_scope: global
layer: stable
rule_version: 0.1.0
bound_objects: all_units
---

# Repository Baseline Rule

This file is the default stable global rule for the repository.

Because this is a stable `g_` rule, every `unit` reads it automatically. A unit must not repeat this file in `rule_refs`. Candidate `g_` rules do not apply automatically; they become default inputs only after promotion to the stable layer.

This file is not a `unit` or `scenario` truth file and does not enter `docs/specs/_status.md`.

## 1. Scope

This rule defines repository-wide defaults that every unit must respect unless the unit truth records an explicit rule exception.

It answers:

1. what the project's formally recognized technology-stack baseline is
2. which shared mechanisms already exist and should be reused by later units
3. which default solution should be preferred for recurring engineering problems
4. which practices are globally forbidden and which exceptions must be explicitly recorded

It does not:

1. describe one unit's internal behavior
2. constrain local implementation details that do not affect repository-wide behavior
3. host candidate proposals
4. replace explicit `b_` rule bindings through `rule_refs`

## 2. Version Semantics

`rule_version` uses `MAJOR.MINOR.PATCH`:

1. `MAJOR`
   - incompatible repository-wide rule change
2. `MINOR`
   - new repository-wide default or compatible extension
3. `PATCH`
   - wording-only clarification that does not change formal rule meaning

When this file's body changes, `rule_version` must change in the same round. If the file is only read, do not change `rule_version`.

## 3. Tech Stack Baseline

1. Primary language:
2. Primary framework / runtime:
3. Primary storage:
4. Cache:
5. Queue / async jobs:
6. Testing stack:

## 4. Reusable Mechanisms

Record only mechanisms that the repository has formally recognized.

1. Configuration management:
2. Logging / auditing:
3. Authentication / authorization:
4. Cache reuse:
5. Scheduling / background jobs:
6. Event or messaging mechanism:
7. ID / unique identifier generation:
8. Retry / degradation strategy:

## 5. Default Selection Rules

Record only the preferred default choice.

1. When a unit needs persistent business data, prefer:
2. When a unit needs short-term shared state or caching, prefer:
3. When a unit needs background async processing, prefer:
4. When a unit needs shared logging, auditing, or tracing, prefer:
5. When a unit needs to reuse an existing mechanism across units, require:
6. When a unit needs a reusable local rule that is not a global default, bind a `b_` rule through `rule_refs`.
7. Multiple units reusing one `b_` rule does not by itself make that rule global.

## 6. Prohibitions And Exceptions

### 6.1 Prohibitions

1. Do not introduce two conflicting primary solutions in parallel for the same class of core capability unless the exception is explicitly recorded.
2. Do not let a unit bypass a formally recognized shared mechanism and rebuild equivalent infrastructure without explanation.
3. Do not present a temporary implementation choice as the repository-wide engineering baseline.

### 6.2 Exceptions

Record exceptions only when the exception is already accepted as repository truth.

1. Exception:

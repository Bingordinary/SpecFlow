---
rule_id: g_rule_repository_baseline
rule_scope: global
layer: stable
rule_version: 0.2.0
---

# Repository Baseline Rule

This file is the default stable global rule for the repository.

Because this is a stable `g_` rule, every `unit` reads it automatically. A unit must not repeat this file in `rule_refs`. Candidate `g_` rules do not apply automatically; they become default inputs only after promotion to the stable layer.

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
8. When a unit's implementation spans multiple modules, prefer one responsibility per module boundary, cut at the natural seams of the behavior domains declared in the unit's acceptance items.
9. Prefer dependency direction that follows the layer order recorded in §5.1: an upper-layer unit may depend on units in the same or lower layers; reverse dependency requires an explicit recorded exception.

### 5.1 Layer Order

The repository's architecture layers, recorded from upper to lower:

1. {layer}
2. {layer}

## 6. Prohibitions And Exceptions

### 6.1 Prohibitions

1. Do not introduce two conflicting primary solutions in parallel for the same class of core capability unless the exception is explicitly recorded.
2. Do not let a unit bypass a formally recognized shared mechanism and rebuild equivalent infrastructure without explanation.
3. Do not present a temporary implementation choice as the repository-wide engineering baseline.
4. Do not create circular dependencies between units. Check method: derive the dependency graph from all current-layer units' `unit_refs`; a cycle (A depends on B while B depends on A, directly or transitively) is a violation. Executed by the agent during unit validate; tooling enforcement is a candidate follow-up.
5. Do not reverse the layer order recorded in §5.1 in `unit_refs` unless the unit truth records an explicit exception. Check method: at validate time, resolve the order from this rule's §5.1 recording and each unit's declared architecture layer from its spec truth (architecture section or design decision records); a `unit_refs` edge from a lower-layer unit to a higher-layer unit is a violation. Units whose spec records no architecture layer are not judged by this prohibition. Executed by the agent during unit validate; tooling enforcement is a candidate follow-up.
6. Do not let a unit's `implementation_surface` entangle responsibilities that the unit's own acceptance items declare as separate behavior domains. Check method: cross-reference the declared behavior domains against the implementation surface layout at validate time.
7. Do not introduce abstractions justified only by future or speculative needs. An abstraction must serve a behavior or constraint recorded in the current unit truth.

### 6.2 Exceptions

An exception is a unit's approved deviation from this rule. Exceptions are recorded in the unit spec frontmatter `rule_exceptions` field with a written reason — never in this rule file. See `specflow/framework/spec_writing_guide.md` §3 for the field contract.

Exception lifecycle:

- Exceptions are recorded only when they are already accepted as repository truth; they are not permanent.
- Each time the unit opens a candidate round, `validate` Check 8 re-evaluates every recorded exception: if it no longer holds (architecture rewritten, rule changed, or reason expired), the exception is reported for removal; if it still holds, the reason is re-examined and the exception is kept.

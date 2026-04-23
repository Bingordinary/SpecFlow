---
version: 0.1.0
---

# System Constraints Spec

> Version note: this document describes the currently effective formal global system constraints. It is not a normal module Spec, does not enter `docs/specs/_status.md`, and by default may be updated only as a linked side product of module `module_promote`. Whenever the body is actually changed, `frontmatter.version` must be incremented in the same round. If the file is only read and not changed, do not change the version.

## 1. Context & Scope

This file answers only:

1. what the project's formally recognized technology-stack baseline is
2. which shared mechanisms already exist and should be reused by later modules
3. which default solution should be preferred for certain engineering problems
4. which practices are globally forbidden and which exceptions must be explicitly recorded

It does not:

1. describe one module's internal state machine
2. constrain function splitting or code-style details inside a single module
3. host draft global proposals from module candidate stages

## 2. Version Semantics

This file uses `MAJOR.MINOR.PATCH`:

1. `MAJOR`
   - incompatible global-constraint change
2. `MINOR`
   - new global default rule, shared mechanism, or compatible extension
3. `PATCH`
   - wording-only clarification that does not change formal constraint meaning

When a module candidate references this file in `Global Constraint Alignment`, it must use:

1. `system_constraints_stable_ref: s_system_constraints@<frontmatter.version>`
2. if the module layer also binds Shared Contract files, it must additionally record `shared_contract_refs` in the same section using the Shared Contract binding contract from `specflow/framework/docs/agent_guidelines/spec_policy.md` Section 6.1; that field does not replace `system_constraints_stable_ref`
3. if the module round proposes a global default-rule change, record it in `system_constraints_change_proposal` inside the module candidate rather than creating an independent system candidate file

## 3. Tech Stack Baseline

> Fill this section with the target repository's real formal baseline. If a module needs to propose new global constraints, it should do so in the module's own candidate instead of creating an independent system candidate file.

1. Primary language:
2. Primary framework / runtime:
3. Primary storage:
4. Cache:
5. Queue / async jobs:
6. Testing stack:

## 4. Shared Mechanisms

> Record shared infrastructure or shared mechanisms that the project has formally recognized. If a mechanism has not yet been formally recognized, do not pretend it already exists here.

1. Configuration management:
2. Logging / auditing:
3. Authentication / authorization:
4. Cache reuse:
5. Scheduling / background jobs:
6. Event or messaging mechanism:
7. ID / unique identifier generation:
8. Retry / degradation strategy:

## 5. Default Selection Rules

> Record only the preferred default choice. Do not pile every historical discussion into this file.

1. When a module needs persistent business data, prefer:
2. When a module needs short-term shared state or caching, prefer:
3. When a module needs background async processing, prefer:
4. When a module needs shared logging, auditing, or tracing, prefer:
5. When a module needs to reuse an existing mechanism across modules, require:
6. When a module needs to reuse shared mechanism text that has not yet been absorbed into the formal global baseline, bind it through Shared Contract instead of double-writing it under a module appendix
7. Multiple modules reusing one Shared Contract does not by itself mean that truth has become a global default rule

## 6. Global Prohibitions / Exceptions

### 6.1 Prohibitions

1. Do not introduce two conflicting primary solutions in parallel for the same class of core capability unless the exception is explicitly registered.
2. Do not let a module bypass a formally recognized shared mechanism and rebuild equivalent infrastructure without explanation.
3. Do not disguise a "temporarily convenient" implementation choice as the formal engineering baseline.

### 6.2 Exceptions

If a module must deviate from this file, at minimum its `Global Constraint Alignment` in candidate must state:

1. what the exception point is
2. why the existing formal constraint does not apply
3. what the impact scope is
4. whether the exception is a temporary bridge or intended to drive a future global upgrade
5. if it also deviates from a Shared Contract, the relation to that shared object must also be stated

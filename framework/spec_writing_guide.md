# Spec Writing Guide

## 1. Purpose

This file defines the writing rules for `specFlow` spec documents in this repository.

It answers five questions:

1. how spec files must be named and where they live
2. which frontmatter fields each object type must record
3. what format acceptance criteria must follow
4. how explanatory and normative content must be organized
5. how this guide relates to other framework governance files

This file is a framework baseline.
Its rules apply automatically to every spec writer and every command that checks spec content.
Project-level standards may tighten these rules through the registered project standards mechanism defined by `project_standards_policy.md`.

---

## 2. Core Principle

A spec document must be:

1. **directly verifiable** — every acceptance item must have a pass condition that can be checked without relying on unwritten context
2. **self-contained in structure** — the required fields (id, target, verification surface, implementation surface, verification method, pass condition) must each be explicitly recorded; the reader should not need to infer them from prose
3. **clearly separated by content type** — explanatory narrative and normative constraints must live in distinct subsections so writers and checkers can distinguish design rationale from fixed rules

---

## 3. File Naming and Location

Spec file paths follow fixed templates defined by `specflow/framework/spec_policy.md` Section 3.

Summary for writers:

| Object | Layer | Path template |
|--------|-------|---------------|
| unit | stable | `docs/specs/units/stable/s_unit_{id}.md` |
| unit | candidate | `docs/specs/units/candidate/c_unit_{id}.md` |
| scenario | stable | `docs/specs/scenarios/stable/s_scenario_{id}.md` |
| scenario | candidate | `docs/specs/scenarios/candidate/c_scenario_{id}.md` |
| rule | stable | `docs/specs/rules/stable/*.md` |
| rule | candidate | `docs/specs/rules/candidate/*.md` |
| repository_mapping | current | `docs/specs/repository_mapping.md` |
| _status.md | current | `docs/specs/_status.md` |

Detailed rules for object identity recording, file name prefixes, typed refs, and truth path resolution are governed by `specflow/framework/spec_policy.md` Section 3 and must be resolved from that file.

---

## 4. Required Fields by Object Type

### 4.1 `unit`

Each current-layer unit truth must record:

2. `rule_refs`

Each candidate-layer unit main file must additionally record these frontmatter fields:

1. `source_basis`
2. `evidence_appendix_ref`

`unit` does not formally record `scenario_refs`.

### 4.2 `scenario`

Each current-layer scenario truth must record:

1. `repository_mapping_ref`
2. `unit_refs`
3. `rule_refs`

Each candidate-layer scenario main file must additionally record these frontmatter fields:

1. `source_basis`
2. `evidence_appendix_ref`

### 4.3 `repository_mapping`

`repository_mapping` must record:

1. current `unit` IDs
2. current `scenario` IDs, or `none`
3. current `rule` IDs

This is repository-structure truth, not lifecycle binding metadata for a command-target object.

### 4.4 `rule`

Each current-layer rule file must record:

1. `rule_id`
2. `rule_scope`
   - `global`
   - `bound`
3. `layer`
4. `rule_version`
5. `bound_objects`
   - `all_units` for stable global rules
   - typed refs or `none` for bound rules and candidate global rules

Rule files may also carry conditional fields (`promotion_owner_unit`, intentional-unbound retention fields) when governance rules require them. Those rules are defined by `specflow/framework/spec_policy.md`.

---

## 5. Testability / Acceptance Criteria Contract

Each current-layer `unit` and `scenario` main Spec must include a `Testability / Acceptance Criteria` section, or an explicitly equivalent acceptance section title.

This section is not a prose-only result description.
It is the formal list of verifiable acceptance items that downstream `check`, `plan`, `verify`, `stable_verify`, and `promote` commands must consume.

Each acceptance item must record these fields:

1. `id`
   - a stable, object-local identifier
   - examples: `ai.model_provider_public_port`, `runtime.task_dispatch_integration`
2. `target`
   - the exact behavior, protocol, boundary, event, storage effect, or external outcome being accepted
3. `verification_surface`
   - exactly one value from the fixed list below
4. `implementation_surface`
   - the concrete package, path set, entrypoint, storage surface, event surface, or manual effect surface that must satisfy the item
5. `verification_method`
   - the command, test, inspection, fixture, external-consumer stub, or manual observation that can prove the item
6. `pass_condition`
   - the concrete observed condition required for this item to pass

The first-version fixed `verification_surface` values are:

1. `public_api`
2. `internal_flow`
3. `error_handling`
4. `eventing`
5. `storage`
6. `integration`
7. `manual_effect`

### Runnable-state rules

1. If an acceptance item cannot be verified in the current repository state, the item must explicitly record `not_runnable_yet` and a non-empty reason.
2. `not_runnable_yet` never counts as `pass`.
3. A command must not silently treat missing test harnesses, missing runtime entrypoints, or unavailable external effects as passed acceptance.
4. `not_runnable_yet` may be used only to avoid making a false pass claim. It does not allow implementation or verification to claim the underlying behavior is complete.

### Surface-specific rules

1. For `verification_surface=public_api`:
   - `implementation_surface` must name the public package, file, or exported contract surface
   - `verification_method` must describe an external-consumer style check
   - `pass_condition` must state that the consumer can satisfy the contract without importing `internal` packages
2. For `verification_surface=integration`:
   - the item must name the runnable integration entrypoint or chain
   - if no such entrypoint exists yet, the item must be marked `not_runnable_yet` with the missing entrypoint reason
   - a broad integration claim must not be counted as accepted only because unit-local pieces pass
3. For `verification_surface=manual_effect`:
   - `verification_method` must name the exact human-observable effect and the observation procedure
   - commands may use `human_verify` only when the remaining uncertainty is truly effect judgment rather than missing executable evidence

### Acceptance item writing rules

1. Do not infer acceptance items from words such as "must", "only", "external", or "replaceable".
2. If a requirement is important enough to block planning, implementation, verification, or promotion, it must appear as an explicit acceptance item.
3. A vague item such as "works", "aligns with design", "is replaceable", or "supports integration" is not sufficient unless the required fields make it directly verifiable.
4. By default, every acceptance item is a current gate item that downstream commands must close.
5. An item is outside the current pass claim only when it explicitly records `not_runnable_yet`, gives the reason, and its `pass_condition` states that it is not a current pass claim.
6. Commands must not infer "key" or "non-key" status from wording, position, length, or apparent importance.
7. Historical stable Specs are not required to be rewritten immediately only because this contract was introduced. They must be brought into this format the next time the object enters `unit_stable_verify`, `scenario` verification, or a fork that touches the acceptance section.

### Example item shapes

```md
- id: ai.model_provider_public_port
  target: External model adapters can implement the AI model provider contract without importing internal packages.
  verification_surface: public_api
  implementation_surface: AgentCore/contracts/model*.go; AgentCore/ports/model_provider.go
  verification_method: External-consumer style compile test with a stub provider that imports only contracts and ports.
  pass_condition: The stub implements ModelProvider and StreamingModelProvider using only public contracts/ports types and no internal import path.
```

```md
- id: runtime.task_dispatch_integration
  target: Runtime dispatch reaches the task execution scenario entrypoint.
  verification_surface: integration
  implementation_surface: Runtime trigger-to-outcome entrypoint
  verification_method: not_runnable_yet
  not_runnable_yet_reason: The repository does not yet expose a complete runtime entrypoint for this chain.
  pass_condition: Not a current pass claim; it becomes runnable only after the runtime entrypoint exists.
```

---

## 6. Section Content Organization

This rule applies to candidate-layer files: `unit` main Specs and appendix files, `scenario` main Specs and appendix files, and `rule` candidate files.

1. Within each section that contains both explanatory content (design rationale, mechanism description, workflow narrative) and normative content (constraints, rules, fixed protocol semantics), these must be separated at the subsection level.
2. Normative content must appear under a clearly marked subheading such as `Fixed Rules` or `约束规则`.
3. Explanatory narrative paragraphs must not embed "must/must not/fixed rules" normative language within the same paragraph or adjacent unseparated paragraphs.
4. The following sections are exempt from this rule:
   - `Terminology` — already uses a structured table format.
   - `Rule Alignment` — already uses a structured reference list format.
   - `Testability / Acceptance Criteria` — already uses a structured acceptance item contract under Section 5 of this guide.
5. This rule does not apply to stable-layer files. Stable-layer truth files follow the existing stable content conventions.

---

## 7. Relationship to Other Framework Documents

1. The formal object identity rules, file path templates, truth path resolution, binding contracts, version semantics, process file rules, and governance lifecycle rules are defined by `specflow/framework/spec_policy.md`.
2. The required frontmatter field `rule_refs` must follow the Rule binding contract defined by `specflow/framework/spec_policy.md` Section 6.1.
3. Commands that check spec content (`unit_check`, `scenario_check`, `rule_new`, and verify commands) read this file as the baseline for spec writing rules.
4. Project-level writing standards may tighten these rules through the registered project standards mechanism defined by `project_standards_policy.md`.

---

## 8. Non-Goals

This file does not:

1. define formal object identity, file path templates, or truth path resolution — those are governed by `spec_policy.md`
2. define binding contracts, version semantics, or process file rules — those are governed by `spec_policy.md`
3. define command lifecycle or governance rules — those are governed by the command policy files under `specflow/framework/commands/`
4. replace `spec_policy.md` as the formal governance source for object families, reading rules, or invalidation
5. create a new command-target lifecycle object or a new governance flow entry

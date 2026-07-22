# Spec Writing Guide

This guide defines the writing contract for specFlow delivery documents.

Files under `specflow/` are framework and delivery documents and are written in English.

Files under `docs/` are project communication documents and are written in Chinese unless a specific delivery artifact requires otherwise.

This file defines formal Spec shape and reference rules, including the semantic authoring baseline in Section 8.

Format compliance does not by itself prove handoff completeness.

## 1. Formal Spec Paths

Unit main Specs:

| kind | layer | path |
|---|---|---|
| unit | stable | `docs/specs/units/stable/s_unit_{id}.md` |
| unit | candidate | `docs/specs/units/candidate/c_unit_{id}.md` |

Rule Specs:

| kind | layer | path |
|---|---|---|
| rule | stable | `docs/specs/rules/stable/{g_or_b}_rule_{id}.md` |
| rule | candidate | `docs/specs/rules/candidate/{g_or_b}_rule_{id}.md` |

`docs/specs/scenarios/**` is not a supported formal Spec path.

## 2. Unit Frontmatter

Each unit main Spec must include these fields:

```yaml
id: {unit}
layer: stable|candidate
version: x.y.z
unit_refs: none
rule_refs: none
```

`unit_refs` may also be a YAML list of unit refs:

```yaml
unit_refs:
  - agent
```

`rule_refs` may also be a YAML list of rule refs:

```yaml
rule_refs:
  - b_rule_example
```

Refs are bare rule names; the ref resolves to the current version.

`evidence_appendix_ref` is an optional frontmatter field referencing an evidence appendix file (e.g., `c_unit_auth_evidence.md`). When present, it records observed implementation behavior that supports the candidate's design decisions. When absent or `none`, the candidate is treated as design-driven (new concept, replacement, or pure design change). The referenced appendix must contain directly readable behavioral truth — not only background, motivation, or patch notes.

## 3. Unit Dependencies

`unit_refs` means the current unit depends on the referenced unit's formal behavior.

It does not mean:

1. the current unit may edit the referenced unit
2. the referenced unit is part of the current unit's ownership

If the body says the unit relies on another unit's official behavior, `unit_refs` must list that unit ref.

## 4. Rule References

Stable global rules are default inputs for every current-layer unit and are not repeated in each unit's `rule_refs`.

`rule_refs` is the only formal consumer binding for bound shared rules.

Rules:

1. `rule_refs` must be in frontmatter
2. no formal `rule_refs` list should be duplicated in the body
3. refs are bare rule names; the ref resolves to the current version
4. refs must be sorted lexically when written as a list
5. `rule_refs: none` means the unit binds no bound shared rule

Rule files must not record consumer truth through `bound_objects`.

## 5. Rule Frontmatter

Each rule Spec must include:

```yaml
rule_id: {rule}
rule_scope: global|bound
layer: stable|candidate
rule_version: x.y.z
```

`promotion_owner_unit` is an optional documentation field. It may be present to indicate which unit owns the promotion decision, but it has no effect on tooling behavior.

`unbound_retention`, `unbound_retention_reason`, and `unbound_retention_owner` may be present when a bound shared rule has no formal current consumers. These fields are used during rule creation and must be removed when formal consumers exist.

### 5.1 Rule Creation

When creating a new rule:

1. Confirm the truth is independent rule truth (not unit-local behavior, binding change, or implementation work).
2. Check that the same formal rule truth is not already present in another rule file or duplicated as unit-local truth.
3. Choose the smallest stable rule boundary. One rule file must carry one coherent shared constraint.
4. A brand-new candidate rule starts at `rule_version: 0.1.0`.
5. If the target bound shared rule already has a stable sibling, derive the current consumer set from current-layer unit `rule_refs`.
6. Create the candidate rule file at `docs/specs/rules/candidate/{rule_id}.md`.
7. If the bound shared rule has no formal current consumers after this write, keep it only when the file explicitly records:
   - `unbound_retention: intentional`
   - `unbound_retention_reason: <why this rule is intentionally independent now>`
   - `unbound_retention_owner: <flow name>`
8. If the bound shared rule has formal current consumers, remove any `unbound_retention` fields.
9. Do not write consumer lists or `bound_objects` into the rule file.

### 5.2 Rule Extraction (Unit → Rule)

When extracting existing unit-local formal truth into a rule:

1. Confirm the request is extraction of existing unit-local formal truth (not new design).
2. Identify the smallest rule object that carries only the shared constraint.
3. Build the complete involved-unit set from current repository truth.
4. If any writeback-required unit is currently stable, stop and create a candidate fork first.
5. Create or update the target candidate rule file. If this is the first file for a new rule object, write `rule_version: 0.1.0`.
6. `promotion_owner_unit` may be written as an optional documentation field if desired.
7. Rewrite each source candidate unit so the extracted truth no longer remains as duplicated unit-local formal truth.
8. Update each affected candidate unit's `rule_refs` and body explanation.
9. Do not write consumer lists or `bound_objects` into any rule file.

### 5.3 Consumer Discovery Constraint

Bound shared rule consumer discovery must use only current-layer unit frontmatter `rule_refs`.
Rule files must not provide consumer truth. `bound_objects` is ignored as a consumer source.

### 5.4 Rule Version Semantics

Agent sets the rule version when editing the rule file. The version must change when the rule body changes (read-only access does not bump version).

Change type determination — compare the candidate version against the current stable version. The first differing segment (MAJOR → MINOR → PATCH) determines the type:

| Type | Condition | Example | Cascade? |
|------|-----------|---------|----------|
| **MAJOR** | Core constraint changes. What was previously allowed is now forbidden, or vice versa. Boundaries tighten or loosen. | `"Must use PostgreSQL"` → `"Must NOT use PostgreSQL"` | Yes |
| **MINOR** | Compatible extension. New exceptions, new options, new clarifications added without changing existing constraint semantics. | `"Must use PostgreSQL"` → `"Must use PostgreSQL, except test environments may use SQLite"` | No |
| **PATCH** | Wording clarification only. Typo fix, sentence rephrase, or any change that does not alter the rule's formal meaning. | `"Must use PostgresSQL"` → `"Must use PostgreSQL"` (spelling fix) | No |

A brand-new rule starts at `0.1.0`. When a rule has no stable version yet (first promotion), no cascade occurs regardless of version.

When editing an existing rule candidate, bump the version deterministically:
- If any existing constraint changes meaning → bump MAJOR
- If new compatible content is added without changing existing meaning → bump MINOR
- If only wording is clarified without meaning change → bump PATCH
- If multiple types of change exist → use the highest (MAJOR > MINOR > PATCH)

## 6. Acceptance Criteria

Each current-layer unit main Spec must include a `Testability / Acceptance Criteria` section or an explicitly equivalent acceptance section.

The section must include structured acceptance items:

```yaml
acceptance_item_set:
  - id: demo.core
    description: Demo behavior is accepted.
    verification_type: testable          # testable | inspectable | reviewable
    verification_surface: internal_flow
    implementation_surface: AgentCore/internal/demo
    verification_method: Go test for demo behavior.
    pass_condition: Demo behavior passes the declared checks.
    not_runnable_yet: no
    evidence_requirements:               # minimum evidence required for this item
      - automated_test_pass
    affects:                             # scope that verify must check globally
      files:
        - internal/demo/handler.go
      appendices: []
      rules: []
      dependencies: []
```

### Acceptance Item Fields

| Field | Required | Description |
|---|---|---|
| `id` | yes | Unique identifier within the item set; used as primary key in process evidence |
| `description` | yes | Plain-language description of this acceptance item |
| `verification_type` | yes | How this item is verified: `testable` (automated test), `inspectable` (file/artifact inspection), `reviewable` (human review) |
| `verification_surface` | yes | Where verification is targeted (e.g. `internal_flow`, `api`, `ui`) |
| `implementation_surface` | yes | Implementation code surface path |
| `verification_method` | yes | How to verify (e.g. "Go test for demo behavior") |
| `pass_condition` | yes | What constitutes a pass |
| `not_runnable_yet` | yes | `yes` or `no` |
| `not_runnable_yet_reason` | recommended | Reason the item is not yet runnable; required when `not_runnable_yet: yes` |
| `target` | recommended | The behavior subject or protocol this item targets (e.g. API endpoint, module boundary, protocol name); used in `acceptance_behavior_fingerprint` calculation |
| `evidence_requirements` | recommended | List of minimum evidence types needed (e.g. `automated_test_pass`, `integration_test_pass`, `old_code_deleted`, `no_remaining_refs`) |
| `affects.files` | recommended | Implementation files that must be verified as part of this item's scope |
| `affects.appendices` | recommended | Appendix names that must be checked |
| `affects.rules` | recommended | Rule names that must be respected |
| `affects.dependencies` | recommended | Stable unit dependency names that must be maintained |

When `verification_type` is `inspectable`, the `evidence_requirements` should specify what inspection evidence is needed (e.g. `old_code_deleted`, `no_remaining_refs`).
When `verification_type` is `reviewable`, human review is the primary verification method; `evidence_requirements` may include `human_review_pass`.

When `verification_type` is `testable`, the acceptance item's `description` and `pass_condition` should be designed so they can be decomposed into a set of unit test scenarios. See `framework/test_decomposition_standard.md` for the decomposition methodology.

The acceptance item ids are used by process evidence. Changing ids invalidates existing process files.

## 7. Appendix Files

Appendix files are support truth for one unit.

They do not replace the unit main Spec.

Appendix ownership is derived from the appendix path and appendix frontmatter, not from a main-Spec index.

Each unit appendix must:

1. use the current path shape for its layer and unit id
2. declare `unit: {unit}` in frontmatter
3. declare `layer: stable|candidate` in frontmatter

When a stable unit with appendix files is forked to candidate, every stable appendix `s_unit_{unit}_{name}.md` must have a corresponding candidate appendix `c_unit_{unit}_{name}.md`.

All appendix files must use the `/appendix/` subdirectory under the layer directory:
- Candidate: `docs/specs/units/candidate/appendix/c_unit_{unit}_{name}.md`
- Stable: `docs/specs/units/stable/appendix/s_unit_{unit}_{name}.md`
The candidate may have additional candidate appendices.

An appendix file may carry an optional `status` field in its frontmatter:

- `status: active` (default) — the appendix participates normally in governance validation and coverage checks.
- `status: exempt` — the appendix is exempt from candidate coverage requirements. A stable appendix with `status: exempt` does **not** require a corresponding candidate appendix, even when the unit has an active candidate round. The tooling skips exempt stable appendices during `CandidateCoverageMismatchesWithExclusions` checks.

The `status` field is validated only when present. Absence is treated as `active`. This field is intended for stable-layer appendices that are valid governance artifacts but not relevant to the current candidate round.

## 8. Authoring Baseline

A formal Spec must make the following clear for the next governance step:

1. the intended user, actor, or caller
2. the unit responsibility and why the unit owns it
3. the entry point or trigger
4. the normal path from input to result
5. the boundaries crossed on that path
6. the data, state, or durable truth each step reads or writes
7. the owner of each read/write responsibility
8. the output artifact or observable result
9. the way failures or unavailable dependencies are exposed
10. the verification surface and success condition

The Spec must close implementation-affecting decisions. The downstream executor must not be forced to choose:
- which object owns a responsibility
- which entry point starts the behavior
- where state or durable truth lives
- how ordered steps connect
- how boundary failures are reported
- what the result shape means
- how acceptance proves the stated responsibility

If a decision is intentionally not made, the Spec must state that boundary and explain why.

### Appendix Handoff

Appendix files may carry detailed truth for one unit but do not weaken the handoff baseline. An appendix used as implementation truth must not contain only background, motivation, principles, or patch notes — it must state the current rule or design as directly readable truth.

## 9. Rule Scope Resolution

Rule scope is resolved from rule truth:
- `rule_scope: global` or id beginning with `g_rule_` → repository-wide rule, applies to every current-layer unit
- `rule_scope: bound` or id beginning with `b_rule_` → bound shared rule, applies only to units listing it in `rule_refs`

When both `rule_scope` and id prefix are present, `rule_scope` in frontmatter takes precedence over id prefix.

Rule files must not store consumer lists. `bound_objects` is not the source of rule consumers. The bound shared rule consumer graph is reconstructed from current-layer unit frontmatter `rule_refs`.

## 10. Dependency Order

```text
unit → rule
stable global rule → unit and rule
rule → unit
unit → unit through unit_refs
```

1. unit truth owns behavior responsibility
2. rule truth owns reusable constraints
3. unit-to-unit dependency is explicit and stable-only

## 11. Prose Content Rules

Spec body prose sections (Description, Responsibility, and any user-defined narrative sections) must not contain code file paths.

All code file path references must be expressed exclusively through the structured fields `implementation_surface` and `affects.files` in the acceptance item set.

This rule does not apply to:
- Spec/governance system paths (e.g., `docs/specs/units/`, `framework/`)
- File paths in code-block examples that serve as illustrations rather than navigation
- Structured field values (`implementation_surface`, `affects.files`)

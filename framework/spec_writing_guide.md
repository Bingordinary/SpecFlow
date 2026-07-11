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
| rule | stable | `docs/specs/rules/stable/s_{g_or_b}_rule_{id}.md` |
| rule | candidate | `docs/specs/rules/candidate/c_{g_or_b}_rule_{id}.md` |

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

`unit_refs` may also be a YAML list of stable unit refs:

```yaml
unit_refs:
  - s_unit_agent@0.6.0
```

`rule_refs` may also be a YAML list of exact rule refs:

```yaml
rule_refs:
  - s_b_rule_example@1.0.0
```

## 3. Unit Dependencies

`unit_refs` means the current unit depends on the referenced stable unit's formal behavior.

It does not mean:

1. the current unit may edit the referenced unit
2. the referenced unit is part of the current unit's ownership
3. the dependency can point to a candidate unit

If the body says the unit relies on another unit's official behavior, `unit_refs` must list that stable unit ref.

## 4. Rule References

Stable global rules are default inputs for every current-layer unit and are not repeated in each unit's `rule_refs`.

`rule_refs` is the only formal consumer binding for bound shared rules.

Rules:

1. `rule_refs` must be in frontmatter
2. no formal `rule_refs` list should be duplicated in the body
3. refs must use exact layer and version
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

`promotion_owner_unit` may be present when one unit owns the promotion decision.

`unbound_retention`, `unbound_retention_reason`, and `unbound_retention_owner` may be present when a bound shared rule has no formal current consumers (see `framework/governance/rules/rule_new.md` Procedure step 8). These fields are used during rule creation and must be removed when formal consumers exist.

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
repository_mapping → unit → rule
stable global rule → unit and rule
rule → unit
unit → unit through stable-only unit_refs
```

1. repository mapping decides path ownership
2. unit truth owns behavior responsibility
3. rule truth owns reusable constraints
4. unit-to-unit dependency is explicit and stable-only

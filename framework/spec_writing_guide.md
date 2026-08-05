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
| unit | stable | `docs/specs/units/stable/unit_{id}.md` |
| unit | candidate | `docs/specs/units/candidate/unit_{id}.md` |

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

Refs are bare unit or rule names; the ref resolves to the current version.

`evidence_appendix_ref` is an optional frontmatter field referencing an evidence appendix file (e.g., `unit_auth_evidence.md`). When present, it records observed implementation behavior that supports the candidate's design decisions. When absent or `none`, the candidate is treated as design-driven (new concept, replacement, or pure design change). The referenced appendix must contain directly readable behavioral truth — not only background, motivation, or patch notes.

The field is the unit-level entry point declaring that the unit has an evidence appendix. The waiver granularity is the acceptance item, not the unit: an acceptance item is evidence-driven when its `affects.appendices` references the evidence appendix, and the design-rationale review is waived for that item only (see `framework/unit_validate_checklist.md` Check 2 Step 2). Items that do not reference the evidence appendix are design-driven and receive full rationale review. Mixed states — some items evidence-driven, others design-driven — are legal and expected during incremental replacement.

Evidence has a defined lifecycle (see §7): it is created by the adoption flow (`framework/operations/adopt.md`), retired section by section as behavior domains are redesigned, and removed entirely when no acceptance item references the evidence appendix — at that point this field is set to `none` and the appendix ends as an empty file (see §7). Zombie, orphan, and residual evidence states are detected by `validate` Check 4 at default severity P1.

`rule_exceptions` is an optional frontmatter field recording this unit's approved deviations from rules that apply to it. Format:

```yaml
rule_exceptions:
  - rule: g_rule_repository_baseline
    reason: "{written justification}"
```

Or `rule_exceptions: none`. Rules:

1. Each referenced `rule` must be a stable global rule or a bound rule listed in this unit's `rule_refs`. A reference to any other rule is invalid.
2. The `reason` must be written justification; exceptions without a reason are invalid.
3. Exceptions are recorded in the unit spec only — never in the rule file. Rule files do not store unit-specific deviations.
4. Exceptions are not permanent. Each time the unit opens a candidate round, `validate` Check 8 re-evaluates every recorded exception: if it no longer holds (architecture rewritten, rule changed, or reason expired), it is reported for removal; if it still holds, the reason is re-examined and the exception is kept.
5. An exception applies only to the unit that records it. It does not exempt any other unit.

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
    runnable: yes
    evidence_requirements:               # minimum evidence required for this item
      - automated_test_pass
    affects:                             # scope that verify must check globally
      files:
        - internal/demo/handler.go
      appendices: []
      rules: []
      dependencies: []
```

> **Note for `testable` items:** When `verification_type` is `testable`, the `description` field must be written as a Gherkin-style Given/When/Then scenario set, not as a single prose sentence. See [§Gherkin-style Description Convention](#gherkin-style-description-convention) for the convention.

### Acceptance Item Fields

| Field | Required | Description |
|---|---|---|
| `id` | yes | Unique identifier within the item set; used as primary key in process evidence |
| `description` | yes | Description of the acceptance item's behavior. For `testable` items, must use Gherkin-style Given/When/Then scenarios (see [§Gherkin-style Description Convention](#gherkin-style-description-convention)). For non-testable items, plain language is acceptable. |
| `verification_type` | yes | How this item is verified: `testable` (automated test), `inspectable` (file/artifact inspection), `reviewable` (human review) |
| `verification_surface` | yes | Where verification is targeted (e.g. `internal_flow`, `api`, `ui`) |
| `implementation_surface` | yes | Implementation code surface path |
| `verification_method` | yes | How to verify (e.g. "Go test for demo behavior") |
| `pass_condition` | yes | What constitutes a pass |
| `runnable` | yes | `yes` or `no` |
| `not_runnable_reason` | recommended | Reason the item is not runnable; required when `runnable: no` |
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

### Gherkin-style Description Convention

This convention applies to acceptance items with `verification_type: testable`.

#### Syntax

The `description` field must contain one or more Gherkin-style behavior scenarios, each following the `Given` / `When` / `Then` pattern:

| Element | Meaning |
|---------|---------|
| `Given` | Precondition — the initial state or context |
| `When` | Action — the trigger or event |
| `Then` | Expected outcome — the observable result |

Multiple scenarios are separated by a blank line. Each scenario should cover one distinct variant (happy path, boundary case, or error path).

The following `.feature` file syntax is **not** used: `Feature:`, `Scenario:`, `Scenario Outline:`, `Examples:` table, or `Background:`.

#### Example

```yaml
  - id: auth.login
    description: |
      Given a registered user with email "user@example.com" and password "ValidP@ss1"
      When the user sends a POST /api/login with correct email and password
      Then the system returns 200 with a JWT token

      Given a registered user with email "user@example.com" and password "ValidP@ss1"
      When the user sends a POST /api/login with wrong password "WrongP@ss1"
      Then the system returns 401 with error code "INVALID_CREDENTIALS"

      Given no user exists with email "nonexistent@example.com"
      When the user sends a POST /api/login with that email and any password
      Then the system returns 404 with error code "USER_NOT_FOUND"
    verification_type: testable
    verification_surface: api
    pass_condition: All three scenarios pass.
```

#### Relationship to Test Decomposition Standard

When `description` is written in Gherkin style, the four-step analysis in `framework/test_decomposition_standard.md` shifts from a generation process to a completeness check: the scenarios are already decomposed; the standard's steps verify no category (happy path, invalid input, business conflict, dependency failure) is missing.

### Acceptance Item Granularity

An acceptance item is the smallest contract unit that carries its own verification lifecycle: verify collects evidence per item id, and promote judges each item's pass/fail independently. Granularity is therefore determined by the verification lifecycle, not by counting behaviors.

#### Three-layer structure

| Layer | Definition | Carrier |
|---|---|---|
| Behavior domain | A semantic cluster of behavior variants around one behavior subject | One acceptance item |
| Scenario | One variant of the domain (happy path, error path, boundary case) | One Given/When/Then block in `description` |
| Test case | One executable check derived from a scenario | Output of `framework/test_decomposition_standard.md` |

One acceptance item covers one behavior domain and contains the domain's complete scenario set (happy path + error paths + boundary cases) inside its `description`. Splitting scenarios of the same domain across items adds verification and evidence tracking overhead without adding information — the variants share the same behavior subject and the same verification route.

#### Behavior domain judgment

Two behavior variants belong to the same behavior domain if and only if all four conditions hold:

1. They describe the same behavior subject (same endpoint, function, state machine, or flow entry point)
2. They share the same `verification_surface`
3. They share the same `implementation_surface`
4. They share the same `verification_type`

Splitting is legitimate when any condition fails. Examples:

- Same endpoint, different `verification_type` (e.g. `testable` API behavior vs `inspectable` config check) — separate items
- Same implementation file, different endpoints — separate items (different behavior subjects)
- Same behavior subject, different error codes or edge cases — one item, multiple scenarios

#### Relationship to validate

`validate` Check 5a uses this standard as its granularity baseline: coverage extraction operates on behavior domains, and over-split detection reports items that satisfy all four conditions above as merge candidates. See `framework/unit_validate_checklist.md` §5a.

## 7. Appendix Files

Appendix files are support truth for one unit.

They do not replace the unit main Spec.

Appendix ownership is derived from the appendix path and appendix frontmatter, not from a main-Spec index.

Each unit appendix must:

1. use the current path shape for its layer and unit id
2. declare `unit: {unit}` in frontmatter
3. declare `layer: stable|candidate` in frontmatter

When a stable unit with appendix files is forked to candidate, every stable appendix `unit_{unit}_{name}.md` must have a corresponding candidate appendix `unit_{unit}_{name}.md`. The `specflowctl fork --unit <name>` command handles this automatically — it copies all active appendix files (skipping `status: exempt`) and applies layer transforms consistently. Always use `specflowctl fork` for this operation; manual copy leaves appendix omission risk.

All appendix files must use the `/appendix/` subdirectory under the layer directory:
- Candidate: `docs/specs/units/candidate/appendix/unit_{unit}_{name}.md`
- Stable: `docs/specs/units/stable/appendix/unit_{unit}_{name}.md`
The candidate may have additional candidate appendices.

**Body references:** The spec body references appendix files and other specs by concept name or file name (e.g. `unit_auth_account_token_claims`), never by layer-prefixed path. Candidate paths break after promote (candidate files are deleted) and stable paths point to the prior-consensus layer during an active candidate round. See §11.

An appendix file may carry an optional `status` field in its frontmatter:

- `status: active` (default) — the appendix participates normally in governance validation and coverage checks.
- `status: exempt` — the appendix is exempt from candidate coverage requirements. A stable appendix with `status: exempt` does **not** require a corresponding candidate appendix, even when the unit has an active candidate round. The tooling skips exempt stable appendices during `CandidateCoverageMismatchesWithExclusions` checks.

The `status` field is validated only when present. Absence is treated as `active`. This field is intended for stable-layer appendices that are valid governance artifacts but not relevant to the current candidate round.

### Evidence Appendix Lifecycle

Evidence appendices are transitional artifacts created by the adoption flow for existing-code onboarding (`framework/operations/adopt.md`). They record observed implementation behavior per behavior domain — one section per behavior domain, each domain corresponding to exactly one acceptance item in the main spec.

The retirement path is incremental, matching how real projects replace old behavior:

1. **Adoption round:** the appendix records all behavior domains; every evidence-driven acceptance item references it via `affects.appendices`; the unit-level `evidence_appendix_ref` points to the file.
2. **Incremental replacement:** when a behavior domain is redesigned in a later iteration, the corresponding acceptance item is converted to design-driven (remove the evidence appendix from its `affects.appendices`, provide design rationale) and the corresponding appendix section is retired (deleted). Other domains keep their evidence references.
3. **Final round:** when no acceptance item references the evidence appendix, retire the last section, promote (the emptied appendix is copied to stable, flushing leftover stable sections), then set `evidence_appendix_ref` to `none`. Do not delete the appendix file before promote — the stable copy cannot be deleted by governance, and deleting only the candidate copy leaves the stable sections in place, re-triggering the orphan finding in every later round. After the final promote the appendix ends as an empty file with no governance effect.

Zombie (item references the appendix with no corresponding section), orphan (appendix section with no corresponding item), and residual (evidence-driven item whose domain has been redesigned) states are detected by `validate` Check 4 and reported at default severity P1.

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

This rule does not apply to:
- Framework governance paths (`framework/`) and validation cache paths (`docs/specs/meta/`) — these describe the governance system itself
- File paths in code-block examples that serve as illustrations rather than navigation

All code file path references must be expressed exclusively through the structured fields `implementation_surface` and `affects.files` in the acceptance item set.

Spec body prose must also not contain layer-prefixed spec paths (`docs/specs/units/candidate/`, `docs/specs/units/stable/`, `docs/specs/rules/candidate/`, `docs/specs/rules/stable/`, or the relative forms `candidate/`, `stable/`) — in prose or in code-block examples. Unlike code file paths, layer-prefixed spec paths break or mispoint in any context: candidate files are deleted on promote, and stable paths point to the prior-consensus layer during an active candidate round. Reference appendix files and other specs by concept name or file name instead (e.g. `unit_auth_account_token_claims`) — appendix file names do not encode layer, so the reference stays valid before and after promote.

This rule does not apply to:
- Structured field values (`implementation_surface`, `affects.files`, `affects.appendices`, `affects.dependencies`) — when a structured field references a spec document, use the stable layer path (it stays valid after promote); candidate-layer spec paths in structured fields are invalid

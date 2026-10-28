# Spec Writing Guide

This guide defines the writing contract for specFlow delivery documents.

Files under `specflow/` are framework and delivery documents and are written in English.

Files under `docs/` are project communication documents and are written in Chinese unless a specific delivery artifact requires otherwise.

This file defines formal Spec shape and reference rules, including the semantic authoring baseline in Section 9 and the abstraction boundary guidelines in Section 14.

Format compliance does not by itself prove handoff completeness.

> **Promise model.** The unit-spec shape, the sole-contract-carrier rule, and
> the gate requirement are defined in `framework/core/promise_model.md`.

==ATOM_BEGIN:promise_gate_requirement==
A unit without `## Promises` is rejected by unit validate, gate-plan, and
promote. A promise without `source`, or an acceptance item that verifies a
promise without `source`, is rejected by the same checks: the projection must
name the Design Narrative block it comes from. The promise format is deployed;
there is no second format.
==ATOM_END:promise_gate_requirement==

## Promise Model (deployed format)

==ATOM_BEGIN:promise_unit_shape==
A unit main spec has exactly three top-level `##` sections, in this order:

1. `## Design Narrative` — why the unit is designed this way; narrative
   subsections use `###`.
2. `## Promises` — the unit's stable contract; structured content only.
3. `## Acceptance Items` — the items that prove each promise; structured
   content only.

Rules: exactly one of each section; no other `##` section is allowed; no
`###` subheading inside `## Promises` or `## Acceptance Items`; one optional
`#` title may precede the sections; frontmatter is not a section; the
acceptance item set is the last content of the spec.
==ATOM_END:promise_unit_shape==

The three top-level sections are `## Design Narrative`, `## Promises`, and
`## Acceptance Items`, in that order. The examples below show the structured
content of the last two sections.

**`## Promises`**

==ATOM_BEGIN:promise_example==
```yaml
promises:
  - id: auth.login_api
    summary: Login returns a token on success and 401 on invalid credentials.
    source: [Authentication model]
    contract:
      operations:
        login:
          inputs:
            username: {type: string, required: true}
            password: {type: string, required: true}
          outputs:
            token: {type: string, required: true}
          errors:
            401: INVALID_CREDENTIALS
          timing: sync
    depends_on: [common.error_schema]
```
==ATOM_END:promise_example==

**`## Acceptance Items`**

==ATOM_BEGIN:acceptance_item_example==
```yaml
acceptance_item_set:
  - id: auth.login_success
    promise: auth.login_api
    source: [Authentication model]
    description: Given valid credentials, when login is called, then 200 + token is returned.
    verification_type: testable
    verification_surface: api
    implementation_surface: auth/login.go
    verification_method: integration test for login success.
    pass_condition: The login success scenario passes.
    runnable: yes
    evidence_requirements: [integration_test_pass]
    affects:
      files: [auth/login.go]
      evidence_files: [auth/login_test.go]
```
==ATOM_END:acceptance_item_example==

- A promise is the stable contract. It has no code range, verification
  method, or consumer list.
- An acceptance item references the promise it proves and keeps the code/test
  scope.
- One promise carries one behavior subject — one endpoint, event, data
  schema, callable surface (an interface method set or one exported function),
  state machine, or standing property (`framework/core/promise_model.md` §Granularity).
- Contract closure compares promise fingerprints: a provider's code, test, or
  narrative change does not reopen its consumers' contract closure. A promise
  contract change reopens its consumers, and the validate and verify gate
  caches follow the same rule.
- Provider units are derived from `depends_on`; a promise depends on another
  unit's contract, not on a promise in the same unit.
- The full schema, contract blocks, fingerprint rules, and `done` state are in
  `framework/core/promise_model.md`.

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

## 2. Unit Division and Project Structure

Unit cuts should map explicitly onto the project's structure instead of existing as an implicit many-to-many relationship between documents and directories.

1. **Prefer explicit mapping.** When dividing work into units, list the project's directory/package structure first and map each responsibility to the directories that carry it. Prefer a one-to-one correspondence between unit and directory; at minimum, record the mapping explicitly at cut time — the adoption flow's cut list carries a directory-mapping column (see `framework/operations/adopt.md` Step 3).
2. **Do not force responsibility cuts onto technical layering.** When the project structure is organized by technical layer (e.g. controller/service), either refactor the structure first or accept cross-directory units. Forcing responsibility-based cuts onto a non-responsibility structure produces mappings that are unreadable to humans and expensive to repair later.
3. **Extract cross-cutting constraints as rules.** Constraint-type content (protocols, component contracts) that spans responsibilities should be extracted as shared rules (see §Rule Extraction) instead of being repeated in every unit document. The adoption flow's structure-review step surfaces this option at cut time (see `framework/operations/adopt.md` Step 2). Shared constraints and file associations are defined by §14.4 File Associations and Shared Agreements.
4. The framework does not force any specific organization: unit cuts are a decision made together with the user, recorded explicitly, and revisited when the structure changes.
5. **Keep a unit to one responsibility; split when it stops being one.** Behavior that shares no design center with the unit — no shared state, no contract the unit and the behavior jointly define, no shared actor journey — is a separate responsibility: create a new unit for it instead of extending the current one. A unit that bundles independent responsibilities gates, evolves, and promotes them together with no shared reason, and inflates every gate run over it. Splitting the unit is the only remedy that reduces what a gate run and a promote decision carry — appendices stay inside the same unit's validation union, so moving content into them changes layering and dependency granularity, not governance weight. Constraints shared between the split parts — protocols and component contracts included — are extracted as rules (item 3), not left as hidden coupling inside one unit. Shared implementation may be associated with several units; reuse public code judgments and keep spec-related judgments separate — see §14.4 File Associations and Shared Agreements. `validate` Check 3 reports a bundled unit as a split candidate (WARNING; see `framework/unit_validate_checklist.md` Check 3 step 6).

## 3. Unit Frontmatter

Each unit main Spec must include these fields:

```yaml
id: {unit}
rule_refs: none
```

The spec layer is encoded by the file path (`docs/specs/units/candidate/` vs `docs/specs/units/stable/`) — no `layer` frontmatter field is declared, and promote performs a pure copy without touching file content.

`rule_refs` may also be a YAML list of rule refs:

```yaml
rule_refs:
  - b_rule_example
```

Refs are bare unit or rule names; the ref resolves to the target's current content.

`evidence_appendix_ref` is an optional frontmatter field referencing an evidence appendix file (e.g., `unit_auth_evidence.md`). When present, it records observed implementation behavior that supports the candidate's design decisions. When absent or `none`, the candidate is treated as design-driven (new concept, replacement, or pure design change). The referenced appendix must contain directly readable behavioral truth — not only background, motivation, or patch notes.

The field is the unit-level entry point declaring that the unit has an evidence appendix. The waiver granularity is the acceptance item, not the unit: an acceptance item is evidence-driven when its `affects.appendices` references the evidence appendix, and the design-rationale review is waived for that item only (see `framework/unit_validate_checklist.md` Check 2 Step 2). Items that do not reference the evidence appendix are design-driven and receive full rationale review. Mixed states — some items evidence-driven, others design-driven — are legal and expected during incremental replacement.

Evidence has a defined lifecycle (see §8): adoption records observed behavior, redesigned domains remove their obsolete evidence sections, and the agent decides whether the final appendix should be deleted through `framework/removal_workflow.md`. Set `evidence_appendix_ref` to `none` when it no longer supports this unit. Zombie, orphan, and residual evidence are detected by validate Check 4 at default severity P1.

`rule_exceptions` is an optional frontmatter field recording this unit's approved deviations from rules that apply to it. Format:

```yaml
rule_exceptions:
  - rule: g_rule_layering
    reason: "{written justification}"
```

Or `rule_exceptions: none`. Rules:

1. Each referenced `rule` must be a stable global rule or a bound rule listed in this unit's `rule_refs`. A reference to any other rule is invalid.
2. The `reason` must be written justification; exceptions without a reason are invalid.
3. Exceptions are recorded in the unit spec only — never in the rule file. Rule files do not store unit-specific deviations.
4. Exceptions are not permanent. Each time the unit opens a candidate round, `validate` Check 8 re-evaluates every recorded exception: if it no longer holds (architecture rewritten, rule changed, or reason expired), it is reported for removal; if it still holds, the reason is re-examined and the exception is kept.
5. An exception applies only to the unit that records it. It does not exempt any other unit.

## 4. Unit Dependencies

A dependency between units is declared on the consumer promise's `depends_on`; the provider unit is derived from the provider promise ID. A promise depends on another unit's contract, not on a promise in the same unit. Dependencies are never inferred from file sharing or prose.

==ATOM_BEGIN:promise_carrier_role==
The `## Promises` block is the unit's sole formal behavior carrier: the place
a consumer reads the unit's formal, externally observable contract. A promise
states what the unit offers; an acceptance item states how the unit proves a
promise; the design narrative states why the unit is designed that way.

Consumers read provider promises; they do not read provider acceptance items,
appendices, or code to learn the contract. A contract statement that no
promise carries is invalid, and an appendix that introduces normative contract
content outside a promise is a finding. An appendix is explanatory support
truth: if it needs its own contract, dependencies, acceptance items, code
scope, or independent verification, it is a unit, not an appendix. A promise
has its own ID and fingerprint but is not a separate formal object family: it
has no independent lifecycle.

Provenance is explicit: every promise, and every acceptance item that verifies
a promise, declares `source:` with the heading text of the Design Narrative
block it projects. `source` records where the contract comes from; it is not
part of the contract fingerprint. A source block change makes the projection
re-checkable without pretending the consumer contract changed.
==ATOM_END:promise_carrier_role==

Everything else in the unit (design narrative, body prose, appendices, background, motivation, design discussion) is not a formal behavior carrier.

**Contract statement definition:** A contract statement declares the unit's externally-observable behavior — behavior a declared consumer must read to use this unit safely: the API surface (endpoints, parameters, status codes), exchanged data formats (field/type/enum names), error codes, protocol names and formats, and timing/consistency guarantees (sync vs async, ordering, timeouts, TTL values a declared consumer must respect). The **external-visibility boundary** — which content is externally observable and which is internal design expression — is defined once, by §15.2; a statement on the internal side of that boundary is not a contract statement even when it states concrete values, because no declared consumer reads it through a promise. `validate` Check 5h enforces the narrative-to-promise relation, and Check 5a applies the boundary to appendix contract content: internal implementation detail declared in the body or an appendix creates no coverage obligation.

Contract statements must be declared in a promise (`validate` Check 5a/5h enforce the narrative and appendix boundaries; see §8). The cross-unit consistency check (`validate` Check 7) reads the dependency unit's promises through the consumer's `depends_on` edges; the run records the provider promise fingerprints, so a contract change is visible to the delta re-run.

Each projection is explicit. A promise declares `source:` with the Design
Narrative block heading(s) it projects; a contract-verifying acceptance item
declares `source:` with the block it proves. The string is the heading text
without `#` markers, exactly as a design report's `Spec basis` names a
`###`-or-deeper narrative block. The
promise stays the contract carrier and the item stays the verification
carrier; `source` records where each came from, so a reviewer can check one
block against one projection instead of searching the whole narrative. The
provider's promise state records the source block content identities; a
source-block edit reopens the provider's own state and requires
re-verification, without reopening consumers unless the contract changes. A
`non_promise` item may omit `source` because it projects no contract. When an
acceptance item has multiple sources, prefer the YAML block-list form; the
inline form is a comma-separated list, so a heading containing a comma must use
the block form.

It does not mean:

1. the current unit may edit the referenced unit
2. the referenced unit is part of the current unit's ownership

If the body says the unit relies on another unit's official behavior, declare the provider promise on the consumer promise's `depends_on`.

## 5. Rule References

Stable global rules are default inputs for every current-layer unit and are not repeated in each unit's `rule_refs`.

`rule_refs` is the only formal consumer binding for bound shared rules.

Rules:

1. `rule_refs` must be in frontmatter
2. no formal `rule_refs` list should be duplicated in the body
3. refs are bare rule names; the ref resolves to the current version
4. refs must be sorted lexically when written as a list
5. `rule_refs: none` means the unit binds no bound shared rule

Rule files must not record consumer truth through `bound_objects`.

## 6. Rule Frontmatter

Each rule Spec must include:

```yaml
rule_id: {rule}
rule_scope: global|bound
```

`promotion_owner_unit` is an optional documentation field. It may be present to indicate which unit owns the promotion decision, but it has no effect on tooling behavior.

A rule may exist without formal consumers. Record useful retention rationale in its body; absence of consumers does not determine deletion. See `framework/removal_workflow.md`.

### 6.1 Rule Creation

When creating a new rule:

1. Confirm the truth is independent rule truth (not unit-local behavior, binding change, or implementation work).
2. Check that the same formal rule truth is not already present in another rule file or duplicated as unit-local truth.
3. Choose the smallest stable rule boundary. One rule file must carry one coherent shared constraint.
4. If the target bound shared rule already has a stable sibling, derive the current consumer set from current-layer unit `rule_refs`.
5. Create the candidate rule file at `docs/specs/rules/candidate/{rule_id}.md`.
6. A rule may remain without consumers. Explain its independent or future value in body prose when useful.
7. Do not write consumer lists or `bound_objects` into the rule file.

### 6.2 Rule Extraction (Unit → Rule)

When extracting existing unit-local formal truth into a rule:

1. Confirm the request is extraction of existing unit-local formal truth (not new design).
2. Identify the smallest rule object that carries only the shared constraint.
3. Build the complete involved-unit set from current repository truth.
4. If any writeback-required unit is currently stable, stop and create a candidate fork first.
5. Create or update the target candidate rule file.
6. `promotion_owner_unit` may be written as an optional documentation field if desired.
7. Rewrite each source candidate unit so the extracted truth no longer remains as duplicated unit-local formal truth.
8. Update each affected candidate unit's `rule_refs` and body explanation.
9. Do not write consumer lists or `bound_objects` into any rule file.

### 6.3 Consumer Discovery Constraint

Bound shared rule consumer discovery must use only current-layer unit frontmatter `rule_refs`.
Rule files must not provide consumer truth. `bound_objects` is ignored as a consumer source.

**Current-layer semantics:** consumer discovery resolves candidate first, stable fallback. This is impact analysis, not deletion approval. Removal checks both physical layers after the planned deletion; publish surviving objects that drop references before final removal. See `framework/removal_workflow.md`.

### 6.4 Spec Removal

Units, bound and global rules, and appendices use `framework/removal_workflow.md`. The agent decides whether their responsibility or constraint ended, then specifies exact targets to `specflowctl remove`. No consumers is not a deletion reason. Normal promote retains rules even when a unit drops them, and fresh reports check status without recommending deletions.

## 7. Acceptance Criteria

Each current-layer unit main Spec must include a `## Promises` section and a `## Acceptance Items` section.

The acceptance section must include structured acceptance items; every contract-verifying item names the promise it proves:

```yaml
acceptance_item_set:
  - id: demo.core
    promise: demo.core
    source: [Demo behavior]
    description: Given a demo input, When the demo behavior runs, Then it is accepted.
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
      evidence_files:
        - internal/demo/handler_test.go
      appendices: []
      rules: []
```

> **Note for `testable` items:** When `verification_type` is `testable`, the `description` field must be written as a Gherkin-style Given/When/Then scenario set, not as a single prose sentence. See [§Gherkin-style Description Convention](#gherkin-style-description-convention) for the convention.

### Acceptance Item Fields

| Field | Required | Description |
|---|---|---|
| `id` | yes | Unique identifier within the item set; used as primary key in process evidence |
| `promise` | required for contract items | The unit-owned promise this item proves (for example `demo.core`). The item's `description` and `pass_condition` state the specific check, not the full promise contract. |
| `source` | required for contract items | Design Narrative block heading(s) this item proves. One or more `###` or deeper heading texts, written exactly as `Spec basis` names them (without `#` markers). A `non_promise` item may omit it. |
| `non_promise` | required for non-contract items | `true` for cleanup, migration, or one-off inspection items that do not verify a promise. Such an item also carries `non_promise_reason`. |
| `description` | yes | Description of the acceptance item's behavior. For `testable` items, must use Gherkin-style Given/When/Then scenarios (see [§Gherkin-style Description Convention](#gherkin-style-description-convention)). For non-testable items, plain language is acceptable. |
| `verification_type` | yes | How this item is verified: `testable` (automated test), `inspectable` (file/artifact inspection), `reviewable` (human review) |
| `verification_surface` | yes | Where verification is targeted (e.g. `internal_flow`, `api`, `ui`) |
| `implementation_surface` | yes | Implementation code surface path: a single repository-relative file or directory. A directory expands to its repository-content files — the files Git tracks plus untracked files that are not ignored; ignored dependencies and build output are not part of the surface. During design-first rounds the value may be the placeholder `<pending>` (the path is not yet known); it must be replaced with the real path before verify — verify reports any leftover `<pending>` as a MISMATCH. The value is a literal anchor: it is matched against the working tree exactly as written, and semicolon-separated lists and wildcard patterns are not interpreted — declare one path per item and list additional files in `affects.files`. A non-`<pending>` value must resolve to at least one real file; `specflowctl validate` Check 3 and verify planning reject a value that does not. A spec document is not an implementation code surface: a value inside the spec tree (`docs/specs/` — a unit or rule spec, an appendix, or a meta record) is rejected by the same Check 3 and verify planning, whether or not it resolves; reference a spec document through `affects.evidence_files` or `affects.appendices` instead, and declare a contract dependency on the consumer promise's `depends_on` (§12). Files participating in this unit behavior may be shared (see §14.4 File Associations and Shared Agreements) |
| `verification_method` | yes | The verification method in one sentence (e.g. "Go test for demo behavior"). No step-by-step test plan and no test-double names — see §15.3 |
| `pass_condition` | yes | What constitutes a pass |
| `runnable` | yes | `yes` or `no` |
| `not_runnable_reason` | recommended | Reason the item is not runnable; required when `runnable: no` |
| `target` | recommended | The behavior subject or protocol this item targets (e.g. API endpoint, module boundary, protocol name) |
| `evidence_requirements` | recommended | List of minimum evidence types needed (e.g. `automated_test_pass`, `integration_test_pass`, `old_code_deleted`, `no_remaining_refs`) |
| `affects.files` | recommended | Implementation files that must be verified as part of this item's scope. Like `implementation_surface`, the field declares implementation code — a spec document is not a valid entry (see the `implementation_surface` row). Files may be shared; behavior dependencies are expressed through the consumer promise's `depends_on`, never through file overlap (see §14.4 File Associations and Shared Agreements) |
| `affects.evidence_files` | recommended | Read-only verification evidence files (typically tests or code owned by another unit) cited to back this item's judgment. They join the run snapshot and the item session's read refs, but they never define implementation participation, quality coverage keys, or operation write scope. Each cited file must exist and must actually carry the behavior or assertion the item claims — the alignment judgment verifies the citation (see `framework/unit_verify_checklist.md` Step 2 → Evidence-carry check). The cited file's own quality belongs to its owner's gates |
| `affects.appendices` | recommended | Appendix names that must be checked |
| `affects.rules` | recommended | Rule names that must be respected |

When `verification_type` is `inspectable`, the `evidence_requirements` should specify what inspection evidence is needed (e.g. `old_code_deleted`, `no_remaining_refs`).
When `verification_type` is `reviewable`, human review is the primary verification method; `evidence_requirements` may include `human_review_pass`.

A contract-verifying item's `description` and `pass_condition` must not contradict the `contract` of the promise it names; `validate` Check 5b owns that comparison. The item states one specific check of the promise, not a second contract.

When `verification_type` is `testable`, the acceptance item's `description` and `pass_condition` should be designed so they can be decomposed into a set of unit test scenarios. See `framework/test_decomposition_standard.md` for the decomposition methodology.

The acceptance item ids are used by process evidence. Changing ids invalidates existing process files. Item ids must not use the reserved final-synthesis key `cross`: a verify plan validates coverage-key uniqueness when it is generated, and an item named `cross` collides with the reserved key, so `gate-plan` rejects the plan before any run state is written (see `framework/verification_scope.md` §Coverage Model → Coverage keys).

Each item's block is also a **structural region**: an acceptance item's immutable verify record pins its own item region (`region:acceptance_item:<id>:<cid>`), and mechanical validation locates the item set structurally. The region runs from the item's `- id:` line to the line before the next item's `- id:` line (trailing blank lines excluded; see `framework/validation_cache.md` §Structural Region Dependencies). Keep every item block a set-level list entry that starts with its own `- id:` line — an unfenced nested line whose trimmed form starts with `- id:` inside another item's field (e.g. a literal example in `description`) would be read as a new item boundary and split that item's region. Fenced code blocks are content and never start an item region. The item set ends at the next `##` heading — the enclosing section's end; `###` and deeper headings never terminate it, so an item separated from the previous one by a `###` subheading remains part of the set (an item placed after the next `##` heading sits outside it and cannot be validated as an item). Moving otherwise unchanged item blocks does not invalidate an item's own region pin — the id is the locator. Set-level content that follows the last item but remains inside the item set (before the next `##` heading) is attributed to the last item's region — keep such content outside the set so item regions stay reorder-stable. Item ids must be non-empty and unique (the `id` requirement above): an empty or duplicated id makes the semantic set unlocatable, so item-region pins fail closed and mechanical validation rejects the spec.

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

`validate` Check 5a uses this standard as its granularity baseline: coverage extraction operates on behavior domains — error paths, boundary cases, and state transitions of the same behavior subject are scenarios of one domain, not separate domains. The correspondence runs both ways: an item whose behavior subject no longer appears as a designed behavior in the spec union is an orphan and is retired in the same round (see §9 Cleanup obligation); `validate` Check 5a fails it. Extraction targets only formal behavior (see `framework/spec_writing_guide.md` §4 Formal behavior carrier): non-constraining narrative (design discussion, motivation, illustrative examples) does not create coverage obligations. See `framework/unit_validate_checklist.md` §5a.

### Contract Substance Baseline

Promises are the formal behavior carrier of a unit (see §4); acceptance items prove them. A cross-unit check reads the provider's promise, so an acceptance item's job is to make that promise falsifiable against code: an item that carries no concrete check leaves the verification empty. The following rules are the **generation standard** — every acceptance item must satisfy all five. `validate` Check 5e enforces them. Concrete values must represent genuine Contract Anchors owned by the unit or exported by dependencies per §14 (Truth Ownership Framework); enumerating private implementation mechanisms, transient test fixtures, or collaborating unit internals is prohibited.

| Rule | Requirement | Rejected form |
|---|---|---|
| S1 — Contract element sufficiency | `description` and/or `pass_condition` must carry at least one concrete contract element: a numeric constraint (timeout, rate, limit), an HTTP status code, a field/type/enum name, an error code, a protocol format, or a timing/consistency assumption | Pure behavior narration with no element: "User can log in", "System handles requests correctly" |
| S2 — Specific-value obligation | Constraints are stated with concrete values: `201` not `2xx`, `5s` not "fast", authentication methods enumerated (`OAuth2 + API Key`) not "multiple methods" | Generalized values: "returns a success code", "responds quickly", "supports various formats" |
| S3 — Scenario completeness | The Gherkin scenario set includes the happy path plus at least one failure or boundary scenario | Only the happy path |
| S4 — Information increment | `pass_condition` carries constraints the `description` does not | `pass_condition` rephrasing the `description` |
| S5 — No template phrasing | No content-free boilerplate | "behaves as expected", "processed correctly", "meets user expectations" |

**Example — rejected:**

```yaml
  - id: auth.login
    description: User can log in with email and password.
    pass_condition: Login succeeds with valid credentials.
```

The item is falsifiable (a broken login fails it) but carries zero contract elements: no status code, no token, no error code. It cannot prove the promise it names: a broken implementation would be indistinguishable from a correct one.

**Example — accepted:**

```yaml
  - id: auth.login
    description: |
      Given a registered user with email "user@example.com" and password "ValidP@ss1"
      When the user sends a POST /api/login with correct email and password
      Then the system returns 200 with a JWT token in the Authorization header

      Given a registered user with email "user@example.com" and password "ValidP@ss1"
      When the user sends a POST /api/login with wrong password "WrongP@ss1"
      Then the system returns 401 with error code "INVALID_CREDENTIALS"
    pass_condition: Returns 200 with JWT on valid credentials; 401 with INVALID_CREDENTIALS on wrong password; token expires after 30 minutes.
```

The item carries concrete elements (status codes, error code, token format, expiry bound) that make the promise verifiable against code.

## 8. Appendix Files

Appendix files are support truth for one unit.

They do not replace the unit main Spec.

Appendix ownership is derived from the appendix path and appendix frontmatter, not from a main-Spec index.

**Contract content belongs to a promise:** the carrier rule in §4 applies. API contracts, data type definitions, error codes, and state machine transitions declared in an appendix are normative contract content and must be moved into a promise in the main spec's `## Promises` section, scoped to externally-observable content per §15.2's external-visibility boundary. `validate` Check 5a reports appendix contract content that has no promise carrier, and Check 5h reports narrative contract statements that no promise carries. Internal implementation detail declared in an appendix is design expression and creates no coverage obligation.

Each unit appendix must:

1. use the current path shape for its layer and unit id
2. declare `unit: {unit}` in frontmatter

Before a unit fork or promote writes any file, the tooling resolves the complete appendix copy set and checks every existing destination's path and frontmatter ownership. The destination must belong to the same unit. A different owner, unreadable file, or missing/inconsistent ownership rejects the operation with the destination path before changing main specs, appendices, candidates, caches or baselines. The same ownership check applies to every appendix; it does not authorize another unit to replace an existing file.

When a stable unit with appendix files is forked to candidate, every stable appendix `unit_{unit}_{name}.md` must have a corresponding candidate appendix `unit_{unit}_{name}.md`. The `specflowctl fork --unit <name>` command handles this automatically — it copies all appendix files. Always use `specflowctl fork` for this operation; manual copy leaves appendix omission risk. After editing, explicitly delete an obsolete draft appendix using the candidate-only removal sequence in `framework/removal_workflow.md`.

All appendix files must use the `/appendix/` subdirectory under the layer directory:
- Candidate: `docs/specs/units/candidate/appendix/unit_{unit}_{name}.md`
- Stable: `docs/specs/units/stable/appendix/unit_{unit}_{name}.md`
The candidate may have additional candidate appendices.

**Body references:** The spec body references appendix files and other specs by concept name or file name (e.g. `unit_auth_account_token_claims`), never by layer-prefixed path. Candidate paths break after promote (candidate files are deleted) and stable paths point to the prior-consensus layer during an active candidate round. See §12.

### Spec Removal

Follow `framework/removal_workflow.md` for unit, rule, and appendix deletion. Appendix replacement removes references in candidate, deletes the old candidate appendix with `--layer candidate`, normally checks and publishes the surviving unit, then explicitly deletes the old stable appendix. Whole-unit removal includes all owned appendices and does not require a publication cycle.

### Evidence Appendix Lifecycle

Evidence appendices are transitional artifacts created by the adoption flow for existing-code onboarding (`framework/operations/adopt.md`). They record observed implementation behavior per behavior domain — one section per behavior domain, each domain corresponding to exactly one acceptance item in the main spec.

The replacement path is incremental, matching how real projects replace old behavior:

1. **Adoption round:** the appendix records all behavior domains; every evidence-driven acceptance item references it via `affects.appendices`; the unit-level `evidence_appendix_ref` points to the file.
2. **Incremental replacement:** when a behavior domain is redesigned in a later iteration, the corresponding acceptance item is converted to design-driven (remove the evidence appendix from its `affects.appendices`, provide design rationale) and the corresponding appendix section is retired (deleted). Other domains keep their evidence references.
3. **Final round:** after the agent confirms the evidence appendix is obsolete, remove its final references and use the candidate-then-stable appendix removal sequence in `framework/removal_workflow.md`.

Zombie (item references the appendix with no corresponding section), orphan (appendix section with no corresponding item), and residual (evidence-driven item whose domain has been redesigned) states are detected by `validate` Check 4 and reported at default severity P1.

## 9. Authoring Baseline

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

### Reader Contract

A Spec's Design Narrative is written for human readers first; Promises and Acceptance Items are the execution contract that consumers and gates read. The Authoring Baseline above defines what must be made clear; this contract defines how a good Spec presents it — as a design narrative that serves four reader audiences, not as a mechanism inventory. A narrative answers "how it works and why"; an inventory only answers "what exists".

| Audience | What the reader must be able to take away |
|---|---|
| **Successor (no code access)** | Restate the full design after reading: how each mechanism works, how data flows, how steps sequence, where state is owned, what happens on failure, and why decisions were made — including alternatives rejected and their reasons. |
| **Engineer** | Know how to implement without reading code: the entry point, what each normal-path step reads and writes, how boundaries are exposed, and the result shape. All design decisions are closed (the must-close list above). |
| **Designer / product manager** | Understand the module's value and position: what problem it solves, its place in the system, its relation to neighboring modules, and what it does not do. This layer must not require code or runtime knowledge. |
| **Acceptance items** | Acceptance items are the means of verifying that the design was implemented. They cover verifiable behavior contracts (written to the code's real capability), but they do not carry design expression. |

**Information layers.** A well-formed Spec presents the unit in four layers, from general to specific:

1. **Value / function layer** — what problem the unit solves, its position in the system, its relationships, and its boundaries (serves the designer / product manager).
2. **Design narrative layer** — mechanisms, data flow, sequencing, state ownership, failure behavior, and decision rationale including rejected alternatives (serves the successor).
3. **Implementation guidance layer** — entry points, step-by-step read/write behavior, boundary exposure, and result shape, with all decisions closed (serves the engineer).
4. **Promise and verification layers** — the stable contract (`## Promises`) and the acceptance items that prove it (`## Acceptance Items`), kept minimal and separate (see §7).

**Document shape (normative order — enforced by §13.10 and `validate` Check 15):** for a unit main Spec:

```text
Design Narrative       → background/value, design narrative, boundaries, and
                         implementation guidance, expressed as ### subsections
Promises               → the stable contract consumers read (see §Promise Model)
Acceptance Items       → the items that prove each promise (see §7)
```

Narrative prose follows §12 Prose Content Rules (no code file paths, no layer-prefixed spec paths); design expression lives in the narrative and structured fields, while the promise block is the formal behavior carrier (see §4).

**Ordering statements and the content boundary.** The required ordered-step account does not conflict with §15.2's exclusion of internal rendering order, prompt block layout, and internal section ordering: §15.2 decides whether an ordering statement may be stated, this contract decides whether the stated content is clear. See §15.2 "Ordering and layout against §9" for where the boundary falls.

**Enforcement (validate Check 10).** This contract is enforced at validate time by the clarity check: one independent session reads the unit's complete spec — the main spec and every appendix — and reports what is unclear, underspecified, or internally contradictory. It judges directly whether the human-readable part states the design clearly enough for its readers to understand it without guessing, and whether any statement conflicts with another; there is no reconstruction step and no second session. An internal contradiction, an unclosed implementation-affecting decision, or a passage a reader cannot understand is a P0/P1 finding. A declarative unit (§14.4's type/contract units) satisfies the check by stating its shape clearly — the public surface and whom it serves, the form of things crossing the boundary, what is frozen, what breaks when changed — not by pretending to have a runtime flow. The check is defined by `framework/unit_validate_checklist.md` Check 10; passing it is a necessary condition, not an upper bound on readability.

### Spec-First Planning and Execution Lifecycle

A Spec is not a post-hoc implementation journal (§15.1 Current truth only); it is the upstream design driver that leads development.

1. **Planning Phase (Proposed Changes Mandate):** When an agent or engineer plans a feature, refactoring, or bug fix, any affected unit's candidate spec must be explicitly declared as a first-class change item before code files. If no candidate spec exists yet, running `specflowctl fork --unit <name>` (or creating the candidate spec) is a declared prerequisite. If the modification is evaluated as a pure internal refactor or performance fix with no changes to external contracts, state transitions, rule constraints, or acceptance criteria, an explicit one-sentence Spec Impact Assessment must state that fact. Silence on spec impact is prohibited.
2. **Execution Phase (Spec-First Order):** When implementing the approved plan, the candidate spec must be updated first (establishing new constraints, updated state flow, and acceptance criteria), and only then may implementation code and tests be written or modified to satisfy the spec.
3. **Verification Phase:** The candidate spec's acceptance items form the verification baseline against which the implementation is verified (`verify@{unit}`).
4. **Cleanup obligation (behavior retirement):** When a candidate round retires or replaces a designed behavior, the same round retires the content that carried it: its acceptance items (an item left behind is the orphan state — `validate` Check 5a fails it), its appendix sections (per `framework/removal_workflow.md`; contract content left without a covering item is failed by `validate` Check 5a's appendix coverage), and the narrative that described the retired behavior's operation. The candidate is current truth, not an accumulation of prior rounds; decision rationale that explains the replacement — including the rejected alternative — is legitimate design expression and stays.

An undeclared removal is not a candidate change: the round's declared scope owns it. The checks close the one-sided failure modes — an item removed while its design remains is failed by coverage extraction, a design removed while its item remains is failed by orphan detection — but a behavior both sides still carry is indistinguishable from an intended behavior, so retirement is a declared-change responsibility, not a document-diff check.

### Appendix Handoff

Appendix files may carry detailed truth for one unit but do not weaken the handoff baseline. An appendix used as implementation truth must not contain only background, motivation, principles, or patch notes — it must state the current rule or design as directly readable truth.

## 10. Rule Scope Resolution

Rule scope is resolved from rule truth:
- `rule_scope: global` or id beginning with `g_rule_` → repository-wide rule, applies to every current-layer unit
- `rule_scope: bound` or id beginning with `b_rule_` → bound shared rule, applies only to units listing it in `rule_refs`

When both `rule_scope` and id prefix are present, `rule_scope` in frontmatter takes precedence over id prefix.

Rule files must not store consumer lists. `bound_objects` is not the source of rule consumers. The bound shared rule consumer graph is reconstructed from current-layer unit frontmatter `rule_refs`.

## 11. Dependency Order

```text
unit → rule
stable global rule → unit and rule
rule → unit
unit → unit through promise depends_on
```

1. unit truth owns behavior responsibility
2. rule truth owns reusable constraints
3. unit-to-unit dependency is explicit; a contract dependency is declared on the consumer promise's `depends_on` and must resolve to a stable provider promise at publication

## 12. Prose Content Rules

`## Design Narrative` subsections (responsibility, mechanism, boundaries, and any user-defined narrative content) must not contain code file paths.

This rule does not apply to:
- Framework governance paths (`framework/`) and validation cache paths (`docs/specs/meta/`) — these describe the governance system itself
- File paths in code-block examples that serve as illustrations rather than navigation

All code file path references must be expressed exclusively through the structured fields `implementation_surface`, `affects.files`, and `affects.evidence_files` in the acceptance item set.

Spec body prose must also not contain layer-prefixed spec paths (`docs/specs/units/candidate/`, `docs/specs/units/stable/`, `docs/specs/rules/candidate/`, `docs/specs/rules/stable/`, or the relative forms `candidate/`, `stable/`) — in prose or in code-block examples. Unlike code file paths, layer-prefixed spec paths break or mispoint in any context: candidate files are deleted on promote, and stable paths point to the prior-consensus layer during an active candidate round. Reference appendix files and other specs by concept name or file name instead (e.g. `unit_auth_account_token_claims`) — appendix file names do not encode layer, so the reference stays valid before and after promote.

This rule does not apply to:
- Path-valued structured fields that may reference a spec document (`affects.evidence_files`, `affects.appendices`) — when one of these fields references a spec document by path, use the stable layer path (it stays valid after promote); candidate-layer spec paths in structured fields are invalid.

`implementation_surface` and `affects.files` are **not** in this exemption. They declare implementation code surfaces (§7), so a spec document is rejected as a value on either field, whether or not it resolves — a spec document is a reference object, and naming it as implementation would send the document's own text into the code-quality review and make the confirmation cache stale the moment the next promote rewrites that path. Reference a spec document through `affects.evidence_files` or `affects.appendices` instead; contract dependencies use the promise's `depends_on`.

## 13. Section Regions

The unit's own main spec must be section-splittable: the content is split by `##` headings — the frontmatter region (file head through the line before the first `##` heading) and one region per `##` heading. Section boundaries define where an `acceptance_item_set` ends (a `##` heading terminates the set); the three-section shape is enforced mechanically by validate Check 15, and region locatability by Check 7. Validation caches no longer declare section regions — cache freshness is whole-file for validate and conclusion-bound for verify (see `framework/validation_cache.md` §Format) — but the structure still keeps the spec machine-locatable and keeps edits contained for the delta re-run.

A verify **design** conclusion pins the spec content it relied on, so the narrative must be addressable at a finer grain than the section: the Design Narrative's own subsections are the anchor unit.

1. **`##`-organized body:** all spec body content must live inside `##` heading sections. The frontmatter region may contain only the YAML frontmatter block, the `#` document title, and blank lines — any other content before the first `##` heading is stray prose (validate Check 1 fails the spec on it; restructure the spec so the region holds only the YAML block, the title, and blank lines).
2. **Unique headings at every level:** heading texts must be unique within the file at all levels (`#` through `######`). A design record locates a narrative block by heading text and a duplicated text cannot be located unambiguously, so validate Check 7 fails a duplicated heading of any level.
3. **`###`-organized Design Narrative:** every line of `## Design Narrative` must live inside a `###` or deeper block — the section heading may be followed only by heading lines, never by prose. A design record's anchors are the narrative block headings and the promise entry ids its report declares (`framework/verification_scope.md` §Verify scope), so prose sitting directly under the section heading would belong to no anchor and be silently uncovered; validate Check 7 fails it. Each `###` block should cover one decision, on the same cohesion reasoning as item 4. A promise's or acceptance item's `source` names one of these blocks by heading text; the block is the projection unit, so keep it cohesive enough that one block carries one contract subject.
4. **One topic per section (cohesion):** each `##` section must cover one semantic topic, so an edit stays contained in one section. If editing any small part of a section tends to touch content across its whole span, the section is too coarse — split it. Cohesion keeps a recorded change set focused, which is what a delta re-run reasons about.
5. **`###` and deeper headings** belong to their `##` section — they never terminate the acceptance item set — and they are the **anchor unit** for design judgments and source projections: a design record pins the `###` (or deeper) block it declares, and a promise or item `source` names the same block, so an edit to a sibling block does not re-run it. A block runs from its heading through the line before the next heading of the same or a shallower level. The shape rule in the Promise Model section defines where `###` is allowed. For region mechanics, a `##` heading ends the `acceptance_item_set`, so keep the item set inside one `##` section.
6. **Naming stability:** headings are part of the region content — the acceptance item set's structural identity changes with any content inside it, so rename deliberately (a rename is a semantic signal, not a cosmetic edit). A narrative-block heading is also its own locator: renaming it makes every design record that declared it re-run, and every promise or item that named it as `source` fail to resolve, because the anchor no longer exists.
7. **Fenced code blocks are content:** `##`-like lines inside ``` or ~~~ fences are not headings — the region mechanism never splits on them, and a fenced `acceptance_item_set:` example never starts the acceptance region. Writing examples that show headings or item sets inside fences is fully legal. A fence cannot cross a `##` heading: close the fence before the next real heading, or the fence's closing line is consumed by the section.
8. **Scope:** this rule applies to the unit's own main spec. Appendix files, rule files, and support appendices need no section structure.
9. **Reserved heading name:** `frontmatter` is a reserved spelling — it names the pre-heading region (heading `""`), so a real `## frontmatter` section cannot be located by section structure (validate Check 7 fails the reserved name). Choose a different heading text.
10. **Fixed section order:** the three-section order and its structural rules are defined once by the Promise Model section (`framework/core/promise_model.md` §Unit spec shape). The acceptance item set remains the **last** content of the file — no heading of any level (`#` through `######`, not only `##`) and no narrative follows it (see §9 Document shape). Set-level content inside the item set is attributed to the last item's region (see §7).

`source` introduces no second anchor type. A source value is the same heading
text a `Spec basis` line uses; it resolves through the same `###`-or-deeper
block locator. Do not add line numbers, generated slugs, or inline
`contract:<id>` anchors: if a heading block is too coarse for a projection,
split it into cohesive blocks first.

A spec that fails the structure (no `##` heading, duplicated heading texts, the reserved `frontmatter` name, stray pre-heading content, Design Narrative prose outside every `###` block, or a section order that violates item 10) fails validate: Check 1 for the structural pre-pass, Check 7 for region locatability, and Check 15 for the three-section shape and order. The fix direction is restructuring the spec per this section.

## 14. Abstraction Boundaries and Anti-Hardcoding Guidelines (The Truth Ownership Framework)

A specification requires determinism to be verifiable, but over-specification destroys resilience and creates brittle, dangling couplings. This section establishes the **Truth Ownership Framework** to resolve the tension between contract substance (the concrete value requirement of §7 and Check 5e) and unacceptable hardcoding.

### 14.1 The Core Principle: Truth Ownership Laws

Whether a concrete value or assertion is an acceptable **Contract Anchor** or an unacceptable **Over-Specification** is determined by who owns the underlying truth:

1. **Ownership Law**: A unit may declare concrete constants, invariants, and boundaries **only** for the behaviors and data structures that it directly owns, and **only when they are externally observable** (§15.2) — a value a declared consumer reads or is bounded by.
   - *Allowed*: The owner unit defines its public HTTP route (`/api/v1/sessions`), return status codes (`201 Created`, `401 Unauthorized`), caller-facing limits (`maxLoops = 20` on a run request, `tokenExpiry = 3600s` on an issued token), and exported serialization keys (`session_id`).
   - *Forbidden*: Non-owner units declaring or duplicating these definitions as private constants or independent tables. An owner declaring an internal value — an internal buffer size, an internal retry count, an internal loop cap — is over-specification under §15.2, not a contract anchor.

2. **Visibility Law (Anti-Shadowing Law)**: Cross-unit contract assertions must anchor strictly to **formal behavior carriers** — the promises exported by collaborating units. A provider's acceptance items and appendices are not its contract.
   - *Allowed*: Referencing exported contract constants (e.g. `contracts.SpanAttr*`), standard protocol events (`DELEGATE_TASK_REQUESTED`), or describing behaviorally what must be exchanged.
   - *Forbidden (Shadow Specifications)*: Non-owner units enumerating internal, volatile, or unexported parameter names/fields of collaborating units.

3. **Behavior vs. Mechanism Law**: Specifications must mandate observable behavior, causal guarantees, and outcome states, not internal execution mechanisms.
   - *Allowed*: Specifying concurrency safety ("thread-safe across concurrent calls"), idempotency, ordering guarantees, or result formats.
   - *Forbidden*: Specifying language-level private data structures (e.g. "must use sync.RWMutex", "must store in map[string]any", specific internal channel buffer sizes).

4. **Environment & Temporal Agnosticism Law**: Specifications must hold across any deployment environment, machine, or CI runner, and must not depend on physical wall-clock accidents.
   - *Allowed*: Causal event-driven or state-driven synchronization conditions (e.g. "when job status transitions to complete"), bounded timeouts with explicit timeout error semantics.
   - *Forbidden*: Local machine absolute paths (`/Users/...`), fixed local IP/ports (`127.0.0.1:8080`), hardcoded environment secrets, or arbitrary physical sleep durations (e.g. "wait 500ms and check status").

### 14.2 Five Hardcoding Anti-Patterns

| Anti-Pattern | Description | Why It Breaks the System | Correct Remedy |
|---|---|---|---|
| **A. Cross-Unit Private Field Enumeration (Shadow Specs)** | An observing/peripheral unit (e.g. `trace`, `logger`, `audit`) hardcodes an attribute table mirroring internal parameters of a core unit (e.g. `agent`, `tool`). | When the core unit refactors away an internal parameter (e.g. `role`), the peripheral spec becomes a regression trap. `verify` divergence analysis tends to recommend `code_gap`, restoring dead code solely to satisfy the shadow spec. | Anchor to exported public constants (`contracts.SpanAttr*`) or describe behavioral requirements without naming private fields. |
| **B. Implementation Mechanism over Behavioral Contract** | Spec dictates internal data structures or concurrency primitives (e.g. "must use sync.RWMutex", "stored in map[string]any"). | Precludes legitimate implementation refactorings and confuses code verification with design validation. | Specify behavioral guarantees (concurrency safety, lookup complexity bounds, idempotent updates). |
| **C. Test Double / Fixture Leakage** | Leaking transient mock names (`test_tool`, `mockRunner`), test fixture IDs, or temporary test doubles into formal acceptance items. | Binds the formal specification to test harness scaffolding rather than production domain behavior. | Express scenarios using domain actor roles and realistic domain inputs. |
| **D. Fragile Temporal Sleep / Wall-Clock Waits** | Specifying arbitrary physical sleep durations (`sleep(500ms)`) as verification pass conditions. | Causes flaky tests on loaded CI systems and fails to verify actual causal event ordering. | Use causal state assertions ("until state reaches X", "upon receiving event Y") with an upper timeout boundary. |
| **E. Environmental & Deployment Hardcoding** | Hardcoding local filesystem paths (`/Users/dev/...`), fixed host ports (`localhost:8080`), or environment-specific credentials. | Breaks across different machines, stages, and platform configurations. | Use relative project-scoped paths, environment configuration references, or abstract placeholders. |

### 14.3 Contract Anchors vs. Unacceptable Hardcoding: Comparison Table

| Dimension | Acceptable & Encouraged (Contract Anchor) | Prohibited (Over-Specification / Hardcoding) |
|---|---|---|
| **API & Endpoints** | `POST /api/v1/sessions` returning `201` | Requiring the internal router to use `chi.Mux` or `gin.Engine` |
| **Cross-Unit Trace / Audit** | "Records delegation target and run ID, using `contracts.SpanAttr*` anchors" | `delegate \| goal, role, child_run_id \| status` mirroring unexported struct fields |
| **Concurrency & State** | "Concurrent calls to `Increment()` are atomic and thread-safe" | "Must synchronize using `sync.RWMutex` guarding `map[string]int`" |
| **Async Operations** | "Waits for `TaskCompleted` event or times out after 10s with `TIMEOUT`" | "Sleeps 500ms then reads database row" |
| **Acceptance Scenarios** | "Given an authenticated admin user" | "Given `mockRunner` configured with `test_tool_v1`" |
| **Domain Limits** | Caller-facing limits: `tokenExpiry = 3600s`, `rateLimit = 100 req/min`, `maxLoops = 20` on a request | Internal limits (internal buffer sizes, internal retry counts) per §15.2, and hardcoding developer local machine config file path `/Users/alice/.config` |

### 14.4 File Associations and Shared Agreements

`implementation_surface` and `affects.files` identify files participating in this unit's behavior, including shared implementations. Directory declarations expand to repository-content files. Several units may declare the same file; sharing does not automatically create a dependency or require an abstraction unit. `affects.evidence_files` is a different relation: it names read-only verification evidence — typically tests or code owned by another unit — that backs the citing item's judgment. An evidence file is never implementation participation of the citing unit, never a quality coverage key, and never operation write scope; its behavior and its quality belong to its owner's gates. The citing item's judgment verifies only that the cited file actually carries the behavior the item claims (see `framework/unit_verify_checklist.md` Step 2 → Evidence-carry check).

Units describe independent responsibilities. Behavior, data meaning and shared agreements still have one authoritative source. Shared constraints belong in rules, consumed through `rule_refs`, rather than being restated in unit specs. Genuine behavior dependencies use the consumer promise's `depends_on`.

Declare paths relevant to the acceptance item. Whether each declaration actually participates in the item's behavior is a spec-vs-code judgment owned by verify Step 3 (`framework/unit_verify_checklist.md`); validate Check 6 judges the declaration's own validity — resolution, ownership, path existence, mutual consistency — and reads no code. Check 7 owns shared-agreement single-source consistency; Check 9 reports the mechanical file-association audit. Unit responsibility boundaries remain the responsibility of the design and scope checks. File overlap alone is not a finding.

`specflowctl surfaces` derives current and stable file associations and displays each associated unit's separate design result alongside reusable public code judgments. See `framework/shared_judgments.md` for verification and immutable record storage.

## 15. Spec Content Boundary

This section is the single authoritative definition of what content belongs in a Spec at all. The validate checks that enforce it (unit: Check 1 — the §15.1 mechanical pre-pass, Check 2 Step 5, Check 5a, Check 5e, Check 5h, Check 7 step 8; rule: Check 6) cite this section and must not restate or redefine it. §14 governs who owns a truth across units; this section governs whether a piece of content belongs in a Spec in the first place — a unit's own internals are not exempt.

**The boundary:** a Spec states only content that is **currently true, externally observable, and not stated elsewhere.** "Externally observable" is judged from the **declaration surface**, never from implementation files: content is externally observable when a **declared consumer** reads it, is bounded by it, or is affected by it. The declaration surface of a unit is its own promises (the `## Promises` block) together with the promises of every unit named by its promises' `depends_on` edges. The declaration surface of a rule is the units that consume it through `rule_refs`. A caller, dependent unit, or other consumer that no declaration names is not a declared consumer and cannot make content observable — the judgment is a claim about what the Spec declares, not about runtime behavior, and the runtime observation that would settle it (the implementation files) is outside the judgment surface. Content that no declared consumer reads, is bounded by, or is affected by is not established as observable by the Spec; §15.2 defines its disposition — prohibited outright when the subject is the unit's own design record or the content's own statement places it on the internal side, and an undeclared boundary requiring a decision otherwise.

### 15.1 Current truth only

The Spec states the current design. It must not carry change history or process traces:

- changelog or revision sections (for example `Version Notes`, `Changelog`)
- review-round narration (gate-fix rounds, revalidate/reverify round summaries)
- removed-behavior narration ("X is removed", "there is no longer a Y", "previously Z")
- migration or replacement history

Change history belongs to version control. When a behavior is retired, its narration is retired with it (§9 Cleanup obligation); the replacement's rationale — including the rejected alternative — is current design expression and stays.

### 15.2 Externally observable only

The Spec states externally observable behavior: content a declared consumer reads to use this unit safely. The judgment is made from the declaration surface defined in §15, never from implementation files, so one document yields one verdict rather than one verdict per reading of the sentence.

**Prohibited — internal implementation detail**, whether of this unit or of another unit:

- internal type and struct definitions, internal field names and layouts
- internal function or method names, internal call sequences and step orders
- internal data structures and their operations
- internal buffer sizes, queue capacities, and thresholds
- internal retry/backoff values and internal timing
- internal rendering order, prompt block layout, internal section ordering
- internal module, package, or file layout; code file paths in narrative
- language-level primitives (`sync.RWMutex`, `map[string]any`, channel primitives)

**Prohibited — the unit's own design record:** content whose subject is the Spec document itself — its sections, headings, or field values, an appendix's layout, or any other property of the design record (for example "the unit's document contains six sections"). The Spec document's structure is a format constraint of this framework, enforced mechanically on the file (validate Check 15, §13), so it is never restated as an acceptance item. The boundary is the record, not the product: a document the unit produces for its consumers is the unit's output, and its observable properties are behavior stated under the categories below.

**Allowed — externally observable:**

- entry points, inputs, and outputs
- public data formats crossing the boundary (field/type/enum names a declared consumer reads)
- error codes and status codes
- protocol names and formats
- timing and consistency guarantees a declared consumer must respect (sync vs async, ordering, timeouts)
- observable limits and configuration a declared consumer sets or is bounded by

**Default dispositions.** The declaration surface decides each case, so no reviewer is asked for a negative existence claim:
1. A declared consumer reads, is bounded by, or is affected by the content → externally observable; it belongs in the Spec.
2. The statement itself places the content on the internal side — it matches a prohibited category above, or uses internal wording such as "internal", "engine-internal", "not exposed", "not open to the host" → prohibited on that wording alone, and no proof that no consumer can observe the value is required.
3. The content states a concrete implementation value, name, layout, ordering, or mapping that fits no prohibited category, is tied to no declared consumer, and is declared on neither side → the boundary is **undeclared**. It is neither passed by default nor failed by inference: it is reported as a `needs_decision` — is this a contract whose consumer the Spec must declare, or an internal detail to retire? Design narrative that describes behavior without asserting such a concrete fact is design expression under §9 and raises no boundary question.

§14.1's Ownership Law governs who may declare a contract anchor across units; it does not exempt a unit from this boundary for its own internals.

**Ordering and layout against §9.** §9's Reader Contract requires the narrative to state how ordered steps connect, while the prohibited list above excludes internal rendering order, prompt block layout, and internal section ordering. The roles separate the two rules: §15.2 governs *what content may be stated*; §9 governs *whether the stated content is clear*. An ordering statement is permitted when it is the observable sequencing guarantee a declared consumer must respect — the ordering entry in the allowed list above — and prohibited when it specifies the internal layout, rendering, or section order through which this unit realizes that sequence. Where a unit's ordered output crosses to a declared consumer that is not a unit (a model reading the prompt, a host reading an event stream), the sequencing guarantee is contract content and is stated behaviorally; the internal block layout that produces it is not.

### 15.3 Not stated elsewhere

- **No test execution recipes and no test doubles.** `verification_method` states the method in one sentence (for example "Go test for demo behavior"). Step-by-step test plans, expected call sequences against stubs, and test-double names belong in the test code, not the Spec (see §14.2 Anti-Pattern C).
- **Each statement is stated once.** The design narrative states how the unit works and why; the promise block states the stable contract consumers read; the acceptance item set states the specific checks that prove each promise. Mechanism narration inside a promise or acceptance item duplicates the design narrative and is removed — the promise keeps the normative contract, the item keeps the verifiable condition.

### 15.4 Section order

`## Design Narrative` comes first, then `## Promises`, then `## Acceptance
Items`. The structural rule is the Promise Model section's shape rule;
§13.10 applies it to section regions.

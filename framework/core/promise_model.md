# Promise Model

> **Status: deployed.** Unit validate, gate-plan, and promote require this
> format.

==ATOM_BEGIN:promise_gate_requirement==
A unit without `## Promises` is rejected by unit validate, gate-plan, and
promote. A promise without `source`, or an acceptance item that verifies a
promise without `source`, is rejected by the same checks: the projection must
name the Design Narrative block it comes from. The promise format is deployed;
there is no second format.
==ATOM_END:promise_gate_requirement==

This document defines the promise layer of a unit spec. It is the design
contract for the spec format, validation, freshness, and `done` state.

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

## Why promises exist

A unit currently carries contract truth, verification detail, and code
bindings in one acceptance-item set. Freshness is therefore compared against
whole files or whole units, and a provider's internal change can invalidate
every consumer.

A promise separates the stable contract from its verification:

- a **promise** states what a unit offers to consumers;
- an **acceptance item** states how the unit proves a promise;
- the **design narrative** states why the unit is designed that way.

Consumers depend on promises, not on units, files, acceptance items, or code.
Contract closure compares promise fingerprints, and the validate and verify
gate caches bind the same fingerprints: a consumer's cache is invalidated by a
provider contract change, not by a provider narrative, acceptance-item, test,
or appendix edit.

### Why there is no second format

The gates bind one contract format. A dual-format window would let a unit
pass validate, gate-plan, and promote while carrying contract truth that no
promise holds, so contract closure — the model's core guarantee — would not
cover it. A project that does not yet carry this format moves its own specs
under the `spec_flow_update` authorization; the framework supplies the current
format, not a migration recipe.

## Unit spec shape

```markdown
---
id: auth
rule_refs: none
---

# Auth

## Design Narrative

### Responsibility

...

## Promises

promises:

## Acceptance Items

acceptance_item_set:
```

This is the section skeleton, not a valid unit: a valid unit adds at least one
promise entry and at least one acceptance item (see the examples below). The
structure-only skeleton still uses the exact marker lines the tooling locates;
`promises: []` and `acceptance_item_set: []` are not structural marker lines.

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

Appendices and rules do not use this shape. An appendix remains subordinate
support truth owned by one unit; a rule keeps its own format.

## Promise block

`## Promises` contains one `promises:` list. Each entry is one promise.

A promise entry is also a **structural region**: it runs from the promise's
`- id:` line through the line before the next promise's `- id:` line, and a
design conclusion that relied on one promise pins that entry
(`region:promise:<id>:<cid>`), located by id rather than by position — so
reordering promises preserves every entry's content identity, while editing
one promise re-runs only the design judgments that declared it.

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

### Fields

| Field | Required | Meaning |
|---|---|---|
| `id` | yes | Stable promise identity. Unit-owned IDs are `<unit>.<name>`; IDs are unique in the repository. |
| `summary` | yes | One-line human description. It is not normative and is excluded from the fingerprint. |
| `source` | yes | Design Narrative block heading(s) this promise projects. One or more `###` or deeper heading texts, written exactly as `Spec basis` names them (without `#` markers). Provenance only: excluded from the contract fingerprint. |
| `contract` | yes | Normative contract content: a combination of the five contract blocks. |
| `depends_on` | no | Promise IDs this promise relies on, owned by another unit. These are graph edges, not part of the fingerprint. |

The promise field set is closed: an unknown field is rejected by promise schema
validation. The contract block vocabulary is also closed, but the combination
is open: a contract states one behavior subject through whichever blocks that
subject needs. An unknown block or an unknown block field is rejected, because
it would enter the fingerprint as contract truth the model never defined.

`depends_on` names a provider promise in another unit. A promise must not
depend on another promise in the same unit: the unit's own contract versions
are recorded together in its promise state, and a same-unit edge is not a
provider dependency.

The `<unit>.<name>` ID carries two roles at once: it is the promise's unique
key, and its prefix is the provider pointer — promote derives the provider
unit from the ID. Renaming a unit therefore changes every promise ID it owns
and every consumer's `depends_on`, and the promise graph check fails closed
on a dangling reference. The string reference is a recorded decision: a
structured `{unit, promise}` reference would add a second format break on
top of the one this model already introduced.

A promise must not contain:

- code paths, code regions, or implementation detail;
- verification methods, test evidence, or acceptance-item fields;
- `verified_by` or a consumer list;
- design rationale or change history.

Verification is declared by acceptance items. Consumers are derived from
dependencies; the provider must not store a consumer list.

### Source

`source` names the Design Narrative block(s) the promise projects. It is the
author's side of the projection: the reviewer's `Spec basis` names the block
it actually read, while `source` names the block the contract claims to carry.
When the basis anchor is a `###`-or-deeper narrative block, both use the same
heading-text spelling and can be compared; a `##` section or a promise entry id
is a context anchor, not a source candidate.

- At least one source is required. A promise without source is a schema
  failure.
- A source is a `###` or deeper heading inside `## Design Narrative`. A `##`
  section, a promise id, an acceptance-item id, or a line number is not a
  source.
- Multiple sources are allowed when one promise carries values from several
  cohesive blocks.
- One block should carry one cohesive contract subject. If a block mixes
  independent subjects, split the block; do not add a second anchor syntax.
- `source` is provenance, not a second contract. It is excluded from the
  contract fingerprint; the promise remains the sole formal behavior carrier.
  Editing `source` alone does not reopen consumers. The provider's promise
  state records each source block's content identity; a source-block edit
  reopens that state and requires the provider to re-verify, while a consumer
  contract reopens only when the `contract` block changes.
- An acceptance item that verifies a promise declares its own `source` the
  same way. A `non_promise` item may omit `source` because it projects no
  contract.

### Contract blocks

A contract states one behavior subject through one or more blocks. The blocks
are the modes in which a consumer can observe the unit; they are not a list of
project types. A new project shape is a new combination of these blocks, not a
new promise kind.

#### `operations`

```yaml
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
```

A map of operation name to operation block. Each operation states a callable
surface: `signature` (the exact textual signature), `inputs` and `outputs`
(field maps), `errors` (error code to meaning), and `timing` (for example
`sync` or `async`). At least one of `signature`, `inputs`, or `outputs` is
required. One interface method set or one exported function is one subject;
two unrelated interfaces belong in separate promises.

#### `data`

```yaml
contract:
  data:
    LoginRequest:
      fields:
        username: {type: string, required: true}
        password: {type: string, required: true}
```

A map of type name to record shape. `fields` is required and non-empty. A wire
type exchanged by an operation may live in the operation's `inputs`/`outputs`;
a type that is itself the subject is a `data` block.

#### `occurrences`

```yaml
contract:
  occurrences:
    order.created:
      payload:
        id: {type: string, required: true}
      delivery: at_least_once
      ordering: per_key
```

A map of event name to occurrence block. The occurrence is provider-initiated:
the consumer does not call it. `payload` is required (an empty field map is
allowed for a payload-free event); `delivery` and `ordering` are non-empty
strings when present.

#### `state_machine`

```yaml
contract:
  state_machine:
    states: [pending, paid, cancelled]
    initial: pending
    transitions:
      - {from: pending, event: pay, to: paid}
```

A single state machine: `states` (non-empty, no duplicates), `initial` (must be
one of the states), and `transitions` (non-empty list of `{from, event, to}`,
with `from` and `to` in `states`).

#### `invariants`

```yaml
contract:
  invariants:
    - safe for concurrent use
    - passwords are never stored in plaintext
```

A non-empty list of standing properties. This is the residual block for a
property that holds across the subject and has no structural shape. It is still
contract content: editing an invariant changes the fingerprint and reopens
consumers, which is correct because the invariant is part of the contract. A
structural fact that an `operations`, `data`, `occurrences`, or `state_machine`
block can carry belongs in that block, not here.

#### Block choice

Choose blocks by what the subject offers, not by the project type. A callable
surface uses `operations`; a record type uses `data`; a provider-initiated
event uses `occurrences`; a lifecycle uses `state_machine`; a standing property
uses `invariants`. A contract may combine blocks when they describe one subject
(for example, an interface with operations and invariants). Bundling unrelated
subjects into one promise remains a granularity finding.

## Granularity

One promise carries exactly one behavior subject: one endpoint or callable
surface, one event, one exchanged data schema, one state machine, or one
standing property. A contract's blocks are facets of that subject; they do not
permit bundling unrelated subjects. A behavior subject is the
same concept `framework/spec_writing_guide.md` §Acceptance Item Granularity
defines for acceptance items — a subject's error codes, boundary cases, and
timing guarantees are its scenarios and stay inside one promise.

Too coarse: a promise bundling several endpoints (one `auth.api` promise
carrying login, token refresh, and session query) recreates
whole-file invalidation one level down — a change to one endpoint's error
code reopens every consumer, including the consumer that depends only on the
session query. Declare `auth.login_api`, `auth.token_refresh_api`, and
`auth.session_query` instead.

Too fine: the 401 error code and the timeout of one endpoint are not two
promises. Scenario variants of one subject share one promise, the same way
behavior variants of one subject share one acceptance item.

A consumer's `depends_on` names the narrowest promise that carries what the
consumer actually reads. A consumer must not pin a bundled promise when a
narrower one exists.

## Acceptance items

An acceptance item is a verification activity. It references the promise it
proves and keeps the evidence/code scope.

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

Rules:

- `promise` is required for an item that verifies a contract;
- an item that verifies a promise declares `source` with the Design Narrative
  block it proves; a `non_promise` item may omit `source`;
- an item that does not verify a promise (cleanup, migration, one-off
  inspection) declares `non_promise: true` and a `non_promise_reason`;
- `pass_condition` states this item's specific check and must not repeat the
  full promise contract;
- code and test scope remain on the item; promises never declare code;
- an item's `promise` must be owned by the same unit;
- a consumer unit depends on another unit's promise through the consumer
  promise's `depends_on`, not through a foreign `promise` reference;
- the gate planner derives provider read refs from `depends_on`.

## Fingerprint and freshness

A promise fingerprint is the SHA-256 of the canonical JSON encoding of:

```json
{"contract": {...}}
```

`summary`, `id`, `depends_on`, and `source` are excluded.

Freshness rules:

- a provider code change does not change a promise fingerprint;
- a provider test/evidence change does not change a promise fingerprint;
- a provider narrative change does not change a promise fingerprint;
- a source declaration is provenance, not contract content: editing `source`
  alone does not change the fingerprint or reopen consumers;
- the promise state records source-block content identities; a source-block
  change reopens the provider's own state (not a consumer contract) and
  requires re-verification;
- a provider promise contract change changes the fingerprint and reopens
  consumers of that promise;
- a dependency promise change reopens the provider's promise only when the
  provider updates its own promise, or when the consumer's pinned dependency
  promise changes.

The provider's own `verify` checks code and evidence against acceptance
items. Consumers do not verify provider code; they pin provider promise
fingerprints. A consumer's item and unit alignment judgments pin the provider
promise fingerprints they read, so a provider contract change re-runs those
judgments instead of leaving a stale verification reusable; the provider spec
text itself is not a freshness pin.

## Done

A unit is `DONE` when:

1. every promise is covered by at least one current acceptance item;
2. every contract-verifying item has current verification evidence for its
   code scope;
3. every dependency promise pin matches the provider promise fingerprint the
   verification read in the layer's resolved scope (see §Layer resolution);
4. the layer's promise state record is present;
5. no blocking finding exists.

Publication adds a stricter condition: every dependency promise pin must also
match the provider's **stable** promise, and that promise must exist in the
stable layer. A consumer is not promoted against a provider contract that is
only a candidate draft. The `fresh` `READY FOR PROMOTE` report applies the same
publication condition. See §Promise state record and the promote workflow.

Global done is the conjunction of all unit done states. A unit is `OPEN` when
its own promises, items, code, or narrative changed; `REOPENED` when a
dependency promise changed; `BLOCKED` when a gate failed.

## Layer resolution

A layer's promise state is computed from that layer's own graph. The provider
scope is fixed by the layer:

- a **candidate** is proposed truth and resolves provider promises in the
  current layer (candidate preferred, stable fallback), so a candidate may
  build on a provider that is still stable-only;
- a **stable** layer resolves provider promises in the published stable layer,
  so a candidate draft of a provider never changes published contract state.

`validate promise-model --layer <layer>` checks the layer's own `depends_on`
edges and the resulting graph. `fresh --unit` and `promote` compare the
recorded pins against that layer's provider fingerprints. A stable
confirmation run records the stable provider fingerprints it read; a candidate
run records the current-layer fingerprints. `promote` then requires the
candidate pin to match both the current provider and the published stable
provider.

## Promise state record

A unit layer records the promise versions it was verified against in:

```text
docs/specs/meta/validation/unit/<unit>/promise_state_<layer>.yaml
```

```yaml
unit: web
layer: candidate
promises:
  - {id: web.session_api, fingerprint: "sha256:..."}
pins:
  - {id: auth.login_api, fingerprint: "sha256:..."}
source_pins:
  - {id: web.session_api, anchor: "3.4.3 Span → 时间线转换", cid: "sha256:..."}
```

`promises` records the unit's own contract versions. `pins` records the
provider promise versions its verification depended on, in the layer's
resolved provider scope (see §Layer resolution): a candidate record pins the
current-layer contracts it read, a stable record pins the published stable
contracts. `source_pins` records the Design Narrative block content identity
each own promise projected at verify time. A source block change reopens the
provider's own state; it does not change the contract fingerprint or reopen
consumers. A record without source pins is stale for a spec that declares
sources and is rebuilt by a full verify — there is no migration path.

A passing unit `verify` finalize records this file automatically. `promote`
validates that the record is current and that every
pinned provider promise is already published at the same contract; it stages
the stable record in the same transaction as the spec, so a failure aborts the
publication. `fork` copies the stable record to candidate.

`fresh` is the single read-only view: its unit summaries include a `done:`
column, and `fresh --unit <name>` reports the complete promise state: schema
status, own promise fingerprints, coverage, graph status, provider pin status,
source-pin status, **contract closure**, gate evidence, and `DONE`. `DONE: YES` requires contract closure `CLOSED` and
fresh validate + verify caches for the layer; otherwise it reports the blocking
gate. `fresh --scope stable` is the global “all stable units done” view. Only a
passing `verify` finalize records the state, so a missing or reopened record is
rebuilt by running `verify@{unit}`.

Contract closure is not code verification; the gate caches remain the
code-evidence source, and both gates now bind provider contracts by promise
fingerprint rather than provider file content.

Provider fingerprints are recorded in three places on purpose: the promise
state record is the durable publication contract (written by verify finalize,
staged to stable in the promote transaction, read by the `fresh` DONE view);
the validate cache's `promise_pins` is the validate gate's input freshness
snapshot; a verify judgment's pin is that conclusion's freshness. Promote's
cross-check (record vs spec vs current layer vs stable layer) enforces their
consistency; a change that adds a pin source must extend that check in the
same change. This is a deliberate denormalization, not drift.

## Validation command

```text
specflowctl validate promise-model --unit <name> [--layer candidate|stable]
```

The command checks the three-section shape, promise schema, coverage, and
dependency graph of the selected layer. A dependency cycle fails every unit
that owns a promise on the cycle; an unrelated unit's contract does not depend
on it and is not blocked. A **unit dependency cycle** also fails every unit on
it: unit edges aggregate every promise of one unit onto another, so two units
can be mutually dependent through different promises (`a.p1` depends on
`b.q1` while `b.q2` depends on `a.p2`) even though the promise graph stays
acyclic. The units are mutually reachable through aggregated promise edges;
this does not require a promise of one unit to reach another promise of the
same unit, because the promise that receives the incoming edge may differ from
the promise that sends the outgoing edge. Units are the management and
promotion unit, so this cycle is a violation, not a reportable structure. A
repository-level identity or read failure (a duplicate promise ID, an
unreadable unit spec) still fails every unit. A unit whose own `## Promises`
section is missing or malformed fails that unit (and any consumer whose
`depends_on` edge stays unresolved); it does not fail unrelated units. The
selected layer also fixes the provider scope the graph resolves in (see §Layer
resolution), so `--layer stable` validates the stable spec's own `depends_on`
edges against published stable providers.

## Change impact matrix

| Change | Provider unit | Consumers |
|---|---|---|
| Code inside an item's scope | Open (verify) | Unchanged |
| Test/evidence | Open (verify) | Unchanged |
| Acceptance item split/merge | Open (verify) | Unchanged |
| Appendix | Open (validate + verify) | Unchanged |
| Narrative | Open (validate + verify) | Unchanged |
| Promise source | Open (validate + verify); provider promise state REOPENED | Unchanged |
| Promise contract | Open (validate + verify) | Reopened |
| Rule | Open (validate/verify) | Rule consumers reopened |
| Global rule | All units open | All units reopened |

A narrative edit also re-runs the provider's design and architecture
conclusions (design conclusions pin the design prose; the architecture
judgment pins the whole main spec), so the provider's verify cache is stale
even though the promise fingerprints and every consumer cache are unchanged.
A source-block edit additionally reopens the provider's promise state through
its `source_pins` and re-runs the acceptance items that declare the block; it
still does not change the contract fingerprint or reopen consumers.

The Consumers column is enforced by both gate caches. A unit validate cache
records a `promise_pins` block of the provider promise fingerprints it read;
a verify conclusion pins the same fingerprints instead of the provider spec
text. A provider narrative, acceptance-item, test, or appendix edit is not a
contract change and leaves the consumer's caches FRESH. A promise contract
change is reported as a promise-level change entry and reopens the consumer's
caches.

## Appendix role

An appendix remains subordinate support truth for one unit:

- path: `docs/specs/units/<layer>/appendix/unit_<unit>_<name>.md`;
- frontmatter: `unit` required;
- no required section structure;
- no acceptance items and no independent lifecycle;
- explanatory detail, examples, and evidence only.

The carrier rule above applies: normative contracts belong to the main spec's
promise blocks. An appendix that needs its own contract, dependencies,
acceptance items, code scope, or independent verification is a unit.

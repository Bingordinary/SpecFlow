# Promise Model

> **Status: deployed.** Unit validate, gate-plan, and promote require this
> format.

==ATOM_BEGIN:promise_gate_requirement==
A unit without `## Promises` is rejected by unit validate, gate-plan, and
promote. The promise format is deployed; there is no second format.
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
cover it. The migration is agent work, executed once per installed project
through `spec_flow_update`, and the one-time cost is lowest while the
installed base is small.

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

promises: []

## Acceptance Items

acceptance_item_set: []
```

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

==ATOM_BEGIN:promise_example==
```yaml
promises:
  - id: auth.login_api
    kind: request_response
    summary: Login returns a token on success and 401 on invalid credentials.
    contract:
      request:
        username: {type: string, required: true}
        password: {type: string, required: true}
      response:
        token: {type: string, required: true}
      errors:
        401: INVALID_CREDENTIALS
      timing:
        mode: sync
    depends_on: [common.error_schema]
```
==ATOM_END:promise_example==

### Fields

| Field | Required | Meaning |
|---|---|---|
| `id` | yes | Stable promise identity. Unit-owned IDs are `<unit>.<name>`; IDs are unique in the repository. |
| `kind` | yes | `request_response`, `event`, `data_schema`, `state_machine`, or `opaque`. |
| `summary` | yes | One-line human description. It is not normative and is excluded from the fingerprint. |
| `contract` | yes | Normative contract content. Shape depends on `kind`. |
| `opaque_reason` | required for `opaque` | Why no structured kind applies. It is justification metadata, excluded from the fingerprint. Only `kind: opaque` may carry it. |
| `depends_on` | no | Promise IDs this promise relies on, owned by another unit. These are graph edges, not part of the fingerprint. |

The promise field set and each kind's contract field set are closed: an
unknown field is rejected by promise schema validation. An extra field is not
part of the deployed format, and a field outside `contract` is not covered by
the fingerprint — it would be contract truth the model never defined.

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

### Contract shapes

`request_response`:

```yaml
contract:
  request: {field: {type: string, required: true}}
  response: {field: {type: string, required: true}}
  errors: {401: INVALID_CREDENTIALS}
  timing: {mode: sync}
```

`event`:

```yaml
contract:
  name: order.created
  payload: {field: {type: string, required: true}}
  delivery: at_least_once
  ordering: per_key
```

`data_schema`:

```yaml
contract:
  fields:
    field: {type: string, required: true, nullable: false}
```

`state_machine`:

```yaml
contract:
  states: [pending, paid, cancelled]
  initial: pending
  transitions:
    - {from: pending, event: pay, to: paid}
```

`opaque`:

```yaml
kind: opaque
summary: Passwords are never stored in plaintext.
opaque_reason: The contract is a security property with no structured surface.
contract:
  text: Passwords are never stored in plaintext.
```

An `opaque` promise is a valid promise when a consumer relies on it and it can
be judged. It is a last resort. `opaque_reason` must name the structured kind
that came closest and the concrete obstacle that disqualifies it — a bare
"no kind fits" does not qualify. The fingerprint still detects an `opaque`
promise's text changes mechanically, but judging what the text contracts is a
human decision, so validate re-confirms every `opaque` promise each round. Any
text change reopens its consumers by default.

## Granularity

One promise carries exactly one behavior subject: one endpoint, one event,
one exchanged data schema, or one state machine. A behavior subject is the
same concept `framework/spec_writing_guide.md` §Acceptance Item Granularity
defines for acceptance items — a subject's error codes, boundary cases, and
timing guarantees are its scenarios and stay inside one promise.

Too coarse: a promise bundling several endpoints (one `auth.api` promise
carrying login, token refresh, and session query) recreates the old
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
{"kind": "...", "contract": {...}}
```

`summary`, `id`, and `depends_on` are excluded.

Freshness rules:

- a provider code change does not change a promise fingerprint;
- a provider test/evidence change does not change a promise fingerprint;
- a provider narrative change does not change a promise fingerprint;
- a provider promise contract change changes the fingerprint and reopens
  consumers of that promise;
- an `opaque` promise text change changes the fingerprint and reopens
  consumers;
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

`validate promise-model --layer <layer>` and the matching `fresh --unit` view
evaluate the layer's own `depends_on` edges and compare its pins against that
layer's provider fingerprints. A stable
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
```

`promises` records the unit's own contract versions. `pins` records the
provider promise versions its verification depended on, in the layer's
resolved provider scope (see §Layer resolution): a candidate record pins the
current-layer contracts it read, a stable record pins the published stable
contracts.

A passing unit `verify` finalize records this file automatically. `promote`
validates that the record is current and that every
pinned provider promise is already published at the same contract; it stages
the stable record in the same transaction as the spec, so a failure aborts the
publication. `fork` copies the stable record to candidate, and whole-unit
removal clears both layers.

`fresh` is the single read-only view: its unit summaries include a `done:`
column, and `fresh --unit <name>` reports the complete promise state: own
promise fingerprints, coverage, graph status, pin status, **contract closure**,
gate evidence, and `DONE`. `DONE: YES` requires contract closure `CLOSED` and
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
on it and is not blocked. A repository-level identity or read failure (a
duplicate promise ID, an unreadable unit spec) still fails every unit. A unit
whose own `## Promises` section is missing or malformed fails that unit (and
any consumer whose `depends_on` edge stays unresolved); it does not fail
unrelated units. The selected layer also fixes the provider scope the graph
resolves in (see §Layer resolution), so `--layer stable` validates the stable
spec's own `depends_on` edges against published stable providers.

## Change impact matrix

| Change | Provider unit | Consumers |
|---|---|---|
| Code inside an item's scope | Open (verify) | Unchanged |
| Test/evidence | Open (verify) | Unchanged |
| Acceptance item split/merge | Open (verify) | Unchanged |
| Appendix | Open (validate + verify) | Unchanged |
| Narrative | Open (validate + verify) | Unchanged |
| Promise contract | Open (validate + verify) | Reopened |
| Rule | Open (validate/verify) | Rule consumers reopened |
| Global rule | All units open | All units reopened |

A narrative edit also re-runs the provider's design and architecture
conclusions (design conclusions pin the design prose; the architecture
judgment pins the whole main spec), so the provider's verify cache is stale
even though the promise fingerprints and every consumer cache are unchanged.

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
- frontmatter: `unit` required, optional `status: active|exempt`;
- no required section structure;
- no acceptance items and no independent lifecycle;
- explanatory detail, examples, and evidence only.

The carrier rule above applies: normative contracts belong to the main spec's
promise blocks. An appendix that needs its own contract, dependencies,
acceptance items, code scope, or independent verification is a unit.

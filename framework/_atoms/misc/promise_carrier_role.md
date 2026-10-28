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

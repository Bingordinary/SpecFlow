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

A promise does not certify its own content: it names its readers — consumers
that exist independently of the promise's own declaration. The named readers
are the units already reached by its `depends_on` edges, or a public boundary
outside the unit graph that the promise itself declares (a public package, a
port, a host or adapter callback). Content whose own wording places it on the
internal side (`framework/spec_writing_guide.md` §15.2) is not made observable
by being carried in a promise — a promise cannot declare a reader for content
its own wording calls internal; the narrative states that boundary as a design
decision.

Provenance is explicit: every promise, and every acceptance item that verifies
a promise, declares `source:` with the heading text of the Design Narrative
block it projects. `source` records where the contract comes from; it is not
part of the contract fingerprint. A source block change makes the projection
re-checkable without pretending the consumer contract changed.

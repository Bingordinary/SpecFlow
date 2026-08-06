# Object Model

This file defines the durable specFlow object model used by active entry files.

## Object Families

specFlow has two formal object families:

1. `unit` - one independently governed engineering responsibility.
2. `rule` - a reusable shared constraint.

`scenario` is not a supported formal object type.

## Unit

A unit owns behavior truth, implementation planning, implementation work, and verification evidence for one engineering responsibility. A unit may describe a local capability, a service slice, or a complete user-visible result chain.

Unit truth lives in:

| Layer | Main Spec | Appendix |
|---|---|---|
| stable | `docs/specs/units/stable/unit_{unit}.md` | `docs/specs/units/stable/appendix/unit_{unit}_{name}.md` |
| candidate | `docs/specs/units/candidate/unit_{unit}.md` | `docs/specs/units/candidate/appendix/unit_{unit}_{name}.md` |

Unit frontmatter records identity, layer, version, `unit_refs`, and `rule_refs`. Appendix files may carry an optional `status` field (`active`, `exempt`, or `retired`) — see `framework/spec_writing_guide.md` §Appendix Files.

## Rule

Rules carry shared constraints.

Rules carry rule_scope in frontmatter — `rule_scope: global` or `rule_scope: bound` — which takes precedence. The id prefix (`g_rule_` for global, `b_rule_` for bound) is the fallback indicator.

- `g_rule_` / `rule_scope: global` → repository-wide rule, applies to every current-layer unit.
- `b_rule_` / `rule_scope: bound` → applies only to units that explicitly list them in `rule_refs`.

Bound rule consumers are derived from current-layer unit `rule_refs`; rule files must not store consumer lists.





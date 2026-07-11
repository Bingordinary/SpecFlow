# Governance Review

Framework review checks whether specFlow governance is coherent and operable.

## Default Mode

Ordinary `spec_flow_review` and governance review use `scoped_review` by default.

Read `framework/governance/review_scope.md` first. Review only the changed files, direct owner files, necessary boundary refs, and minimal convergence refs needed to answer the request.

`scoped_review` does not use `_governance_review/` run-state, baseline slice tables, dynamic slice tables, or score-state tables by default.

Plain exact `spec_flow_design_review` is not scoped.
It always delegates to `framework/spec_flow_design_review.md` and runs the default full-scope design-baseline review.
There is no narrowed or scoped `spec_flow_design_review` mode.

## Entries

The exact review forms are `spec_flow_review`, `spec_flow_review:full`, and `spec_flow_design_review`. These are referenced by the keyword table below.

0. If the entry expression does not match an exact review form, match against this keyword table in order:
   - If the expression contains "mechanism audit" or "framework correctness" → treat as `spec_flow_review`, default `scoped_review`.
   - If the expression contains "governance review" or "governance audit" → treat as `spec_flow_review`, default `scoped_review`.
   - If the expression contains "design review" or "design audit" or "design quality" → treat as `spec_flow_design_review`.
   - If none match → stop per Section "Unrecognized Entry".

1. `spec_flow_review` checks mechanism correctness.
2. `spec_flow_design_review` checks design quality and agent operability.

Plain exact `spec_flow_review` routes through this file first and remains scoped.
The only full-scope mechanism review entry is exact `spec_flow_review:full`.

Plain exact `spec_flow_design_review` routes through this file first, then directly delegates to `framework/spec_flow_design_review.md`.

When the entry is exact `spec_flow_review:full`, `framework/spec_flow_review.md` is the deep-audit owner.

If the entry expression does not match any recognized review entry (exact forms or keyword-matched entries from rule 0), stop and report that the entry is unrecognized. Do not silently fall through to a default or guess the caller's intent.

### Unrecognized Entry

The paragraph above is the Unrecognized Entry stop condition.

## Review Layout

Deep-audit review tooling uses a single fixed layout: `source_repo`.

- framework inputs live under local `framework/`
- templates live under local `templates/`
- tooling lives under local `tooling/`
- project-instance compatibility reviews template bootstrap compatibility
  under `templates/docs/specs/` and does not require real project-instance
  `docs/specs/` files

### Layout-Aware Path Resolution

Files in `framework/` reference paths that resolve to the `source_repo` layout.
`docs/specs/` paths are template bootstrap files present under `templates/docs/specs/`;
project-instance files at `docs/specs/` do not exist in this layout and must be
treated as informational references (agents must check path existence before reading
and skip non-existent paths with a documented note).

Review layout applies to exact `spec_flow_review:full` mechanism deep audit and
every `spec_flow_design_review`. It does not widen ordinary `spec_flow_review`
scoped review.

## Active Scope

Default `spec_flow_review` scoped review uses the layered framework structure:

1. `core/`
2. `governance/`
3. `operations/`
4. guidance skills
5. templates
6. tooling contracts and source

`framework/spec_flow_review.md` is the mechanism deep-audit owner and is not ordinary default context for scoped review.
`framework/spec_flow_design_review.md` is the ordinary owner for every `spec_flow_design_review`.

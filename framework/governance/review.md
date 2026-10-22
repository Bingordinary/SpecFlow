# Governance Review

Framework review checks whether specFlow governance is coherent and operable.

## Default Mode

Ordinary `spec_flow_review` and governance review use `scoped_review` by default.

Read `framework/governance/review_scope.md` first. Review only the changed files, direct owner files, necessary boundary refs, and minimal convergence refs needed to answer the request.

`scoped_review` does not use `meta/governance_review/` run-state, baseline slice tables, dynamic slice tables, or score-state tables by default.

Plain exact `spec_flow_design_review` is not scoped.
It always delegates to `framework/spec_flow_design_review.md` and runs the default full-scope design-baseline review.
There is no narrowed or scoped `spec_flow_design_review` mode.

## Entries

The exact review forms are `spec_flow_review`, `spec_flow_review:full`, and `spec_flow_design_review`. These are referenced by the keyword table below. Plain `spec_flow_review` followed by one or more repository-relative paths is the scoped entry form for explicit file or directory review; the paths are the scope inputs.

0. If the entry expression does not match an exact review form or the scoped path-args form, match against this keyword table in order:
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

If the entry expression does not match any recognized review entry (exact forms, the scoped path-args form, or keyword-matched entries from rule 0), stop and report that the entry is unrecognized. Do not silently fall through to a default or guess the caller's intent.

### Unrecognized Entry

The paragraph above is the Unrecognized Entry stop condition.

## Review Layout

Deep-audit review tooling uses a single fixed layout: `source_repo`.

- framework inputs live under local `framework/`
- templates live under local `templates/`
- tooling lives under local `tooling/`

### Layout-Aware Path Resolution

Files in `framework/` reference paths that resolve to the `source_repo` layout.
`docs/specs/` paths are project-instance paths; project-instance files at
`docs/specs/` do not exist in this layout and must be
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

## Repair Recommendation Soundness

A `recommended fix` that removes, weakens, or changes the behavior of an existing mechanism must name the premise that makes the change safe, and the review must verify that premise before the finding is reported.

The premise is one of three classes, each with its own verification:

1. executable behavior — the premise states what a command, tool, or code path does.
   Verify by executing it on a constructed input and recording the observed result.
2. repository text — the premise states what a rule document says, or that two texts agree or conflict.
   Verify by citing the exact governing lines, using the P3 fact-anchor proof shapes (absence, contradiction, convention deviation; `framework/severity_policy.md` §10).
3. future executor behavior — the premise states what an executor will do after the change.
   This class is not verifiable. The finding must not present such a fix as settled: it must state that the repair path is unresolved, name the assumption, and leave the decision to the user.

The verification result is part of the finding's `evidence`. A class 1 or class 2 premise without its verification makes the finding incomplete. A class 3 premise is the unresolved-repair case: the finding keeps its `recommended fix` field, states that the repair path is unresolved, and names the assumption for the user to decide.

This check is agent work and is deliberately not tooling-enforced: choosing the verification target and judging class 2 and class 3 premises is semantic judgment, which `framework/tooling_execution_policy.md` §5 forbids tooling from performing.

Execution position: the same step as the severity validation — scoped review conclusion stage (`framework/governance/review_scope.md`), full-scope procedure step 10 (`framework/spec_flow_review.md`), and design review procedure step 13 (`framework/spec_flow_design_review.md`).

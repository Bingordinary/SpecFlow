# Governance Review Scope

Mechanism governance review has two modes: `scoped_review` and `deep_audit`.
`spec_flow_design_review` has one mode: default full-scope design-baseline review owned by `framework/spec_flow_design_review.md`.

`scoped_review` is the default for ordinary framework changes, explicit file or directory review, recently changed governance material, and plain `spec_flow_review`.
It is not a `spec_flow_design_review` mode.

`deep_audit` is explicit for mechanism review. Use it only for exact `spec_flow_review:full`.

Plain `spec_flow_design_review` must not be narrowed through this file.
It delegates to `framework/spec_flow_design_review.md` and uses the design review run-state, baseline slices, score state, and pass gate defined there.

## `scoped_review`

`scoped_review` checks the smallest governance surface that can answer the user's question.

Required inputs:

1. the user request or changed files.
2. direct owner files for those inputs.
3. boundary refs needed to verify handoff, authority, or tooling agreement.
4. minimal convergence refs when the changed surface crosses another owner.

When the user does not specify a review scope (no file paths, no changed files, no specific question), default to reviewing git working directory changes (`git diff` against HEAD). If no working directory changes exist, default to the most recent commit (`git log -1`). This default prevents unnecessary escalation to `deep_audit` and aligns with the expectation that a plain `spec_flow_review` entry checks current uncommitted work.

It must not use `meta/governance_review/` run-state by default.
It must not require baseline slice table, dynamic slice table, score-state table, or full-scope run-state startup.

Output fields:

1. `scope mode: scoped_review`
2. `input refs`
3. `owner refs`
4. `boundary refs`
5. `checks performed`
6. `findings`
7. `conclusion`

If the output does not explicitly report all seven output fields, the review is not complete.

Every real finding in a scoped review must be written as a short repairable story before trace details.
The story must let a new maintainer understand the execution path, expected rule behavior, actual gap, possible wrong next step, and smallest correct repair.

Do not present a raw field dump as the user-facing finding.
Lists such as `background`, `what happened`, `impact`, and `recommended fix` may be used only as private drafting aids or trace details; they do not satisfy the user-facing finding requirement by themselves.

Every real finding in a scoped review must still include:

1. `severity: P0|P1|P2|P3`
2. `blocking: yes|no`
3. `evidence`
4. `recommended fix`

Severity and blocking may appear in a status line after the story.
Evidence and trace details should appear after the reader can already understand the problem.

Severity uses `framework/severity_policy.md`.
The `severity` and `blocking` fields in every scoped finding together satisfy the Section 6 baseline from `framework/severity_policy.md` (background, what happened, impact, recommended fix, why minimal fix, blocking). The story format covers background, what happened, impact, and why minimal fix (via the "smallest correct repair" requirement); the explicit fields cover severity, blocking, evidence, and recommended fix.
Severity describes harm level; it does not replace explicit blocking status.
`P0` and `P1` are normally blocking.
`P2` and `P3` are normally non-blocking unless the finding explains why the current scope must stop.

The conclusion may be:

```text
scoped_pass | scoped_blocked | needs_deep_audit
```

`scoped_pass` means only that the reviewed scope is coherent under the named inputs.
It must not claim full governance-baseline pass or full design-baseline pass.
`scoped_blocked` means at least one in-scope finding has `blocking: yes`.

## `deep_audit`

`deep_audit` preserves the full-scope review machinery.

Use `deep_audit` only when the user explicitly requests exact `spec_flow_review:full` for mechanism correctness.

For mechanism correctness, deep audit is owned by `framework/spec_flow_review.md`.
For design quality, plain `spec_flow_design_review` is already full-scope and is owned by `framework/spec_flow_design_review.md`.

Deep audit may use `meta/governance_review/` run-state and existing `specflowctl review run-*` tooling.
`spec_flow_design_review` uses that run-state tooling by default.

Deep-audit tooling for exact `spec_flow_review:full` uses the `source_repo` layout:

- framework root: `framework/`
- template root: `templates/`
- tooling root: `tooling/`
- compatibility input: template bootstrap compatibility under `templates/docs/specs/`, not real project `docs/specs/`

## Escalation

A scoped review must stop with `needs_deep_audit` when:

1. the requested conclusion is repository-wide or baseline-wide.
2. the affected owner set cannot be bounded from the user request and changed files.
3. the review discovers a cross-owner risk that cannot be answered by minimal convergence refs.
4. the user asks for resumability, slice state, score state, or run-state tooling.

Do not silently widen a scoped review into deep audit.
Do not silently narrow `spec_flow_design_review` into scoped review.

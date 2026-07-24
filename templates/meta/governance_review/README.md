# Governance Review Run State

This directory stores process files for explicit full-scope mechanism reviews and every `spec_flow_design_review`.

This file records review process state. Terms like baseline slice, dynamic slice, stale, and slice set are defined in the owning review policy under `framework/governance/review.md`.

Rules:

1. Files under `meta/governance_review/` are process files.
2. They are not Specs and are not behavior sources of truth.
3. Explicit deep-audit `spec_flow_review` uses:
   - `meta/governance_review/spec_flow_review.md`
4. Every `spec_flow_design_review` uses:
   - `meta/governance_review/spec_flow_design_review.md`
5. Scoped `spec_flow_review` and ordinary scoped governance reviews do not use full-scope run state by default.
6. A run-state file records review progress, slice status, input fingerprints, findings, blocked reason, and resume position.
7. A `spec_flow_design_review` run-state file may also record score-state progress, but it must not decide score correctness.
8. A run-state file must not define review rules.
9. The owning review policy defines:
   - when the file is created
   - when it may be reused
   - when it must be deleted and recreated
   - which fields and status values are legal
10. Full-scope review mechanical writes are maintained by `specflowctl review run-* --flow <review_flow> --layout auto|installed|source`, including the run-state skeleton, UTC timestamps, review layout, baseline slice table, input fingerprints, structural validation, and stale refresh.
11. Tooling maintains only mechanical fields. It does not maintain review judgment.
12. Slice pass decisions, question scores, score basis, finding content, finding severity, non-blocking optimization content, hard-blocker judgment, and the final conclusion remain review-executor judgments under the owning review policy.
   - `spec_flow_review` final conclusions are `pass | blocked`.
   - `spec_flow_design_review` final conclusions are `pass | pass-with-optimization | blocked`.
13. When an explicit full-scope mechanism review or any `spec_flow_design_review` resumes a run-state file, it must refresh slice fingerprints and mark changed slices as stale before continuing.
14. A full-scope review result must not claim a passing conclusion until every required baseline slice and dynamic slice is closed by the owning review policy.
   - For `spec_flow_review`, the only passing conclusion is `pass`.
   - For `spec_flow_design_review`, the passing conclusions are `pass` and `pass-with-optimization`.
15. Each review flow has only one fixed run-state file. Starting a new full-scope review deletes the previous fixed file before the new run state is written.
16. `meta/governance_review/` must not contain per-flow run-state subdirectories.
17. Each run-state file records `review_layout`.
18. `installed_project` layout reviews real project-instance compatibility under `docs/specs/`.
19. `source_repo` layout reviews template bootstrap compatibility under `templates/docs/specs/` and does not require real project-instance `docs/specs/` files.

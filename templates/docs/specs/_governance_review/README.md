# Governance Review Run State

This directory stores process files for resumable full-scope governance reviews.

Rules:

1. Files under `_governance_review/` are process files.
2. They are not Specs and are not behavior sources of truth.
3. Default full-scope `spec_flow_review` uses:
   - `docs/specs/_governance_review/spec_flow_review/{review_run_id}.md`
4. Default full-scope `spec_flow_design_review` uses:
   - `docs/specs/_governance_review/spec_flow_design_review/{review_run_id}.md`
5. Narrowed governance reviews do not use full-scope run state by default.
6. A run-state file records review progress, slice status, input fingerprints, findings, blocked reason, and resume position.
7. A `spec_flow_design_review` run-state file may also record score-state progress, but it must not decide score correctness.
8. A run-state file must not define review rules.
9. The owning review policy defines:
   - when the file is created
   - when it may be reused
   - when it must be deleted and recreated
   - which fields and status values are legal
10. Full-scope review mechanical writes are maintained by `specflowctl review run-* --flow <review_flow>`, including the run-state skeleton, UTC timestamps, baseline slice table, input fingerprints, structural validation, and stale refresh.
11. Tooling maintains only mechanical fields. It does not maintain review judgment.
12. Slice pass decisions, question scores, score basis, finding content, finding severity, hard-blocker judgment, and the final `pass | blocked` conclusion remain review-executor judgments under the owning review policy.
13. When a full-scope review resumes a run-state file, it must refresh slice fingerprints and mark changed slices as stale before continuing.
14. A full-scope review result must not claim `pass` until every required baseline slice and dynamic slice is closed by the owning review policy.

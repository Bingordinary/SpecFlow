# Verify Results

This directory stores candidate verification results for formal command-target objects and the minimal stable acceptance coverage summaries preserved after promotion.

Rules:

1. `_verify_result/{object_type}/{object}.md` should exist only for formal objects whose current `verify` result still covers current truth and current verification scope.
2. `_verify_result/stable/{object_type}/{object}.md` stores the acceptance coverage summary for a promoted stable version.
3. These files are not formal Specs and are not behavior sources of truth.
4. The current round allows only these command-target object types here:
   - `unit`
   - `scenario`
5. Candidate `Verify Result Snapshot` must use at least these fixed fields:
   - `object_type`
   - `object_ref`
   - `gate`
   - `decision`
   - `allow_next`
   - `next_command`
   - `blocking_summary`
   - `coverage_summary`
   - `truth_layer_ref`
   - `truth_file_ref`
   - `truth_version_ref`
   - `truth_fingerprint`
   - `rule_snapshot`
   - `verification_scope_ref`
   - `acceptance_item_set`
6. Candidate verify results must include `acceptance_item_evidence_matrix`:
   - each row must correspond to one acceptance item `id`
   - each row status must be one of `pass`, `fail`, `partial`, `not_checked`, or `not_runnable_yet`
   - passing tests that do not prove the corresponding `pass_condition` must not be recorded as `pass`
7. Object-owned snapshot extensions are fixed by object type:
   - `unit` additionally records `unit_appendix_snapshot` and `rule_snapshot`
   - `scenario` additionally records `repository_mapping_snapshot`, `unit_snapshot`, and `rule_snapshot`
8. `gate` must equal the formal `verify` command for the current object family.
9. If current candidate truth, current implementation, current bound snapshots, or current acceptance item set change after verification, the candidate verify result becomes outdated.
10. Consumers must validate bindings, snapshots, and acceptance item coverage, not just existence.
11. Snapshot fields in this file must use the fixed definitions from `specflow/framework/process_snapshot_contract.md`.
12. `verification_scope_ref` minimally means that the current verify result still covers current candidate truth plus the current object state that was verified in that round.
13. `scenario_verify` must report `affected_units`, but those units do not become implicitly repaired or complete from that report alone.
14. When commands explain why verification must continue, fall back, or reroute, they must use the standardized `fallback_reason_code` taxonomy first and then add natural-language explanation.
15. Stable acceptance coverage summaries must be written to:
   - `docs/specs/_verify_result/stable/unit/{unit}.md`
   - `docs/specs/_verify_result/stable/scenario/{scenario}.md`
16. Stable acceptance coverage summaries must record at least:
   - `object_type`
   - `object_ref`
   - `stable_truth_file_ref`
   - `stable_truth_version_ref`
   - `stable_truth_fingerprint`
   - `promotion_verify_result_ref`
   - `acceptance_item_set`
   - `acceptance_item_coverage_summary`
   - `key_evidence_source_refs`
17. A stable acceptance coverage summary records the evidence that closed promotion. It is not a claim that later code still aligns with stable truth.

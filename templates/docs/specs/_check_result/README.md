# Candidate Check Results

This directory stores candidate-chain pass gates for formal command-target objects.

Rules:

1. `_check_result/{object_type}/{object}.md` should exist only for formal objects that passed their current-layer `check` command and still hold a valid candidate-chain gate.
2. These files are not formal Specs and are not behavior sources of truth.
3. The current round allows only these command-target object types here:
   - `unit`
   - `scenario`
4. `Check Result Snapshot` must use at least these fixed fields:
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
   - `acceptance_item_set`
5. Object-owned snapshot extensions are fixed by object type:
   - `unit` additionally records `unit_appendix_snapshot` and `rule_snapshot`
   - `scenario` additionally records `repository_mapping_snapshot`, `unit_snapshot`, and `rule_snapshot`
6. `gate` must equal the formal `check` command for the current object family.
7. A `check` command creates or overwrites this file only when the result is `pass`.
8. When `check` does not pass, it must not write a failed-state file; if an old gate is no longer valid, delete it.
9. `acceptance_item_set` must record the accepted item `id`, `verification_surface`, and whether the item is `not_runnable_yet`.
10. As soon as current candidate truth changes, `_check_result/{object_type}/{object}.md` becomes outdated.
11. It also becomes outdated when current baseline bindings, current object-owned snapshots, or the current acceptance item set drift from current truth.
12. Consumers must validate bindings, snapshots, and acceptance item sets, not just existence.
13. Snapshot fields in this file must use the fixed definitions from `specflow/framework/process_snapshot_contract.md`.
14. `_check_result/{object_type}/{object}.md` carries only the current pass-gate snapshot. It does not carry failed fallback records.
15. When commands explain why this pass gate cannot be consumed or why the object must fall back, they must use the standardized `fallback_reason_code` taxonomy first and then add natural-language explanation.

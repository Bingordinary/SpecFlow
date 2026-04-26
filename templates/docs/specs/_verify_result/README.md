# Candidate Verify Results

This directory stores candidate verification results for formal command-target objects.

Rules:

1. `_verify_result/{object_type}/{object}.md` should exist only for formal objects whose current `verify` result still covers current truth and current verification scope.
2. These files are not formal Specs and are not behavior sources of truth.
3. The current round allows only these command-target object types here:
   - `unit`
   - `scenario`
4. `Verify Result Snapshot` must use at least these fixed fields:
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
   - `verification_scope_ref`
   - `system_constraints_file_ref`
   - `system_constraints_version_ref`
   - `system_constraints_fingerprint`
5. Object-owned snapshot extensions are fixed by object type:
   - `unit` additionally records `unit_appendix_snapshot` and `shared_contract_snapshot`
   - `scenario` additionally records `repository_mapping_snapshot`, `unit_snapshot`, and `shared_contract_snapshot`
6. `gate` must equal the formal `verify` command for the current object family.
7. If current candidate truth, current implementation, or current bound snapshots change after verification, the file becomes outdated.
8. Consumers must validate bindings, not just existence.
9. Snapshot fields in this file must use the fixed definitions from `specflow/framework/process_snapshot_contract.md`.
10. `verification_scope_ref` minimally means that the current verify result still covers current candidate truth plus the current object state that was verified in that round.
11. `scenario_verify` must report `affected_units`, but those units do not become implicitly repaired or complete from that report alone.
12. When commands explain why verification must continue, fall back, or reroute, they must use the standardized `fallback_reason_code` taxonomy first and then add natural-language explanation.

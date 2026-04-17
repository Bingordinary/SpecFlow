# Candidate Check Results

This directory stores candidate-chain pass gates produced after candidate closure passes.

Rules:

1. `_check_result/{module}.md` should exist only for modules that have passed `cand_check` and still hold a valid candidate-chain pass result.
2. These files are not formal Specs and are not behavior sources of truth.
3. Each file carries the latest valid pass snapshot for the current candidate, not a failed-review record.
4. `Check Result Snapshot` must use fixed fields:
   - `module`
   - `gate`
   - `decision`
   - `allow_next`
   - `next_command`
   - `blocking_summary`
   - `coverage_summary`
   - `spec_layer_ref`
   - `spec_file_ref`
   - `spec_version_ref`
   - `spec_fingerprint`
   - `module_appendix_snapshot`
   - `system_constraints_stable_file_ref`
   - `system_constraints_stable_version_ref`
   - `system_constraints_stable_fingerprint`
   - `shared_contract_snapshot`
5. The fixed fields above are the framework snapshot baseline. A command may add command-owned project extension fields only when that command's governance file explicitly allows them, and those extension fields must not replace, weaken, or redefine the fixed fields above.
6. For `cand_check`, `project_review_extensions` is allowed only when `specflow/framework/docs/agent_guidelines/commands/cand_check.md` explicitly permits that container for the consumed project-local review surface.
7. `gate` is always `cand_check`.
8. `next_command` is always `cand_plan`.
9. `cand_check` creates or overwrites this file only when the result is `pass`.
10. When `cand_check` does not pass, it must not write a failed-state file; if an old pass gate is no longer valid, delete it.
11. As soon as candidate truth changes, `_check_result/{module}.md` becomes outdated.
12. It also becomes outdated when the formal global baseline binding or Shared Contract snapshot no longer matches current truth.
13. `spec_fork` must delete the previous round's `_check_result/{module}.md`.
14. `cand_promote` must delete the corresponding `_check_result/{module}.md`.
15. Consumers must validate bindings, not just existence.
16. This README is also constrained by `specflow/framework/docs/agent_guidelines/candidate_handoff_contract.md`.
17. Snapshot fields in this file must use the fixed definitions from `specflow/framework/docs/agent_guidelines/process_snapshot_contract.md`.
18. `_check_result/{module}.md` carries only the current pass-gate snapshot. It does not carry failed fallback records.
19. When commands explain why this pass gate cannot be consumed or why the module must fall back, they must use the standardized `fallback_reason_code` taxonomy first and then add natural-language explanation.

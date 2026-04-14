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
   - `prompt_adequacy_review_required`
   - `prompt_adequacy_decision`
   - `prompt_adequacy_summary`
   - `spec_layer_ref`
   - `spec_file_ref`
   - `spec_version_ref`
   - `spec_fingerprint`
   - `system_constraints_stable_file_ref`
   - `system_constraints_stable_version_ref`
   - `system_constraints_stable_fingerprint`
   - `shared_appendix_snapshot`
5. `gate` is always `cand_check`.
6. `next_command` is always `cand_plan`.
7. `cand_check` creates or overwrites this file only when the result is `pass`.
8. When `cand_check` does not pass, it must not write a failed-state file; if an old pass gate is no longer valid, delete it.
9. As soon as candidate truth changes, `_check_result/{module}.md` becomes outdated.
10. It also becomes outdated when the formal global baseline binding or Shared Appendix snapshot no longer matches current truth.
11. `spec_fork` must delete the previous round's `_check_result/{module}.md`.
12. `cand_promote` must delete the corresponding `_check_result/{module}.md`.
13. Consumers must validate bindings, not just existence.
14. This README is also constrained by `specflow/framework/docs/agent_guidelines/candidate_handoff_contract.md`.
15. `_check_result/{module}.md` carries only the current pass-gate snapshot. It does not carry failed fallback records.
16. When commands explain why this pass gate cannot be consumed or why the module must fall back, they must use the standardized `fallback_reason_code` taxonomy first and then add natural-language explanation.
17. The semantics of `prompt_adequacy_review_required`, `prompt_adequacy_decision`, and `prompt_adequacy_summary` are additionally constrained by `specflow/framework/docs/agent_guidelines/prompt_gate_contract.md`.

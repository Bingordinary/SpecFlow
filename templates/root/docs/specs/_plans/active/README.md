# Candidate Active Plans

This directory stores the consumable implementation plans for candidate progression.

Rules:

1. Each unit normally has one `active/{unit}.md`.
2. These files are not formal Specs and are not behavior sources of truth.
3. Each active plan file carries these formal planning sections:
   - `Execution Surface Plan`
   - `Retirement Targets`
   - `Verification Targets`
4. Each active plan file also carries:
   - `Implementation Tasks`
   - current-round progress, blockers, and verification focus
5. `Execution Surface Plan` must be organized around the changed execution surfaces of the round rather than one forced whole-unit path.
6. Each execution surface must name its current known path, target path, and first cutover slices.
7. `Retirement Targets` must name which legacy paths, helpers, patches, wrappers, or equivalent dependencies should stop being required for the round to close.
8. `Verification Targets` must name which retirement goals `unit_verify` must explicitly prove.
9. `Implementation Tasks` should be organized as closeable execution slices rather than one undifferentiated task block.
10. Each slice should at minimum name:
   - objective
   - file scope
   - dependencies
   - verification action
   - done condition
   - current status
11. Each active plan file must also record:
   - `spec_file_ref`
   - `spec_version_ref`
   - `spec_fingerprint`
   - `unit_appendix_snapshot`
   - `system_constraints_stable_file_ref`
   - `system_constraints_stable_version_ref`
   - `system_constraints_stable_fingerprint`
   - `shared_contract_snapshot`
12. `active/{unit}.md` does not carry the gate decision of `unit_check` or the promotion decision of `unit_verify`.
13. `unit_plan` writes or updates `active/{unit}.md` only when the round is `plan-ready`.
14. If planning is still blocked or still inside a decision checkpoint, `unit_plan` must keep updating `draft/{unit}.md` instead of rewriting `active/{unit}.md`.
15. `unit_impl` may keep writing progress, blockers, verification notes, and retirement progression, but must not rewrite binding fields.
16. For each advanced slice, `unit_impl` should write back at minimum:
   - `execution_surface`
   - `cutover_result`
   - `retirement_result`
   - `verification_note`
17. `unit_verify` still requires a current valid `active/{unit}.md`.
18. `unit_verify` must consume `Execution Surface Plan`, `Retirement Targets`, and `Verification Targets` as part of the round's formal verification input.
19. If candidate truth changes, the active plan becomes outdated.
20. It also becomes outdated when formal global baseline bindings or Shared Contract bindings drift.
21. Once outdated, the flow must go back to `unit_check` first and then re-run `unit_plan`.
22. `unit_fork` must delete the previous round's `active/{unit}.md`.
23. `unit_promote` must delete the corresponding `active/{unit}.md`.
24. When `Candidate=no`, `active/{unit}.md` should not remain.
25. This README is also constrained by `specflow/framework/docs/agent_guidelines/candidate_handoff_contract.md`.
26. Snapshot fields in this file must use the fixed definitions from `specflow/framework/docs/agent_guidelines/process_snapshot_contract.md`.

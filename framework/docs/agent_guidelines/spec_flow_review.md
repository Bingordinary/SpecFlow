# Spec Flow Review

## 1. Purpose

`spec_flow_review` reviews the governance mechanism itself.

It answers four questions:

1. whether the governance rule set still closes the full Spec Flow
2. whether the tooling layer still matches the rule layer
3. whether shared-governance and impact-reconciliation semantics still converge with the command core
4. whether the repository may still claim one coherent governance baseline

Plain input `spec_flow_review` means the default governance-baseline review defined in this file unless the user explicitly narrows scope.

This flow does not review business truth by default.
It reviews the mechanism that governs business truth.
It does not prove that the current governance design is sensible, humane, or worth using as designed.
That judgment belongs to `spec_flow_design_review`.

## 2. Default Scope

The default scope includes:

1. framework governance rules
   - `specflow/framework/docs/agent_guidelines/*.md`
2. command rules
   - `specflow/framework/docs/agent_guidelines/commands/*.md`
3. template-side process and state contracts
   - `specflow/templates/root/docs/specs/_status.md`
   - `specflow/templates/root/docs/specs/_check_result/README.md`
   - `specflow/templates/root/docs/specs/_plans/README.md`
   - `specflow/templates/root/docs/specs/_plans/draft/README.md`
   - `specflow/templates/root/docs/specs/_plans/active/README.md`
   - `specflow/templates/root/docs/specs/_verify_result/README.md`
4. template entry files
   - `specflow/templates/root/AGENTS.md`
   - `specflow/templates/root/GEMINI.md`
   - `specflow/templates/root/CLAUDE.md`
5. entry registry and project-standard governance files
   - `specflow/framework/docs/agent_guidelines/entry_index_registry.md`
   - `specflow/framework/docs/agent_guidelines/project_standards_policy.md`
   - `specflow/framework/docs/agent_guidelines/project_standard_create.md`
   - `specflow/templates/root/docs/project_standards/_registry.md`
   - `docs/project_standards/_registry.md`
   - the active registered project-local standards in scope
6. tooling contract and tooling source
   - `specflow/framework/docs/agent_guidelines/tooling_execution_policy.md`
   - `specflow/tooling/README.md`
   - `specflow/tooling/cmd/**/*.go`
   - `specflow/tooling/internal/**/*.go`

Default scope must explicitly include:

1. the shared-governance rule set
   - at minimum `shared_ops.md`, `shared_new.md`, `shared_extract.md`, `shared_bind.md`, `shared_topology.md`, `shared_sync.md`, and `shared_escape.md`
2. the impact-reconciliation rule set
   - at minimum `impact_sync_policy.md`, `process_snapshot_contract.md`, `recovery_policy.md`, template `_status.md`, and the process README files
3. the tooling execution contract set
   - at minimum `tooling_execution_policy.md`, `specflow/tooling/README.md`, and the in-scope tooling source files

If any one of those three coverage sets is missing from a default-scope review, that review is not complete and must not issue `pass`.

## 3. Review Blocks

For the default governance-baseline review, the execution-local `review_plan` must use exactly these fixed review blocks:

1. `review_and_command_core`
   - `spec_flow_review.md`
   - `spec_policy.md`
   - `command_policy.md`
   - `implementation_change_policy.md`
   - `git_policy.md`
   - `severity_policy.md`
   - `checkpoint_protocol.md`
   - `project_spec_policy.md`
   - `flow_policy.md`
   - `commands/*.md`
2. `shared_governance`
   - `shared_ops.md`
   - `shared_new.md`
   - `shared_extract.md`
   - `shared_bind.md`
   - `shared_topology.md`
   - `shared_sync.md`
   - `shared_escape.md`
3. `impact_reconciliation`
   - `impact_sync_policy.md`
   - `shared_sync.md` only where it defines handoff into `impact_sync`
   - `process_snapshot_contract.md`
   - `recovery_policy.md`
   - `specflow/templates/root/docs/specs/_status.md`
   - `specflow/templates/root/docs/specs/_check_result/README.md`
   - `specflow/templates/root/docs/specs/_verify_result/README.md`
   - `specflow/templates/root/docs/specs/_plans/draft/README.md`
   - `specflow/templates/root/docs/specs/_plans/active/README.md`
4. `process_and_state_contracts`
   - `candidate_handoff_contract.md`
   - `downgrade_policy.md`
   - `process_snapshot_contract.md`
   - `recovery_policy.md`
   - `specflow/templates/root/docs/specs/_status.md`
   - `specflow/templates/root/docs/specs/_check_result/README.md`
   - `specflow/templates/root/docs/specs/_plans/README.md`
   - `specflow/templates/root/docs/specs/_plans/draft/README.md`
   - `specflow/templates/root/docs/specs/_plans/active/README.md`
   - `specflow/templates/root/docs/specs/_verify_result/README.md`
5. `entry_and_project_local_extension`
   - `entry_index_registry.md`
   - `project_standards_policy.md`
   - `project_standard_create.md`
   - `specflow/templates/root/AGENTS.md`
   - `specflow/templates/root/GEMINI.md`
   - `specflow/templates/root/CLAUDE.md`
   - `specflow/templates/root/docs/project_standards/_registry.md`
   - `docs/project_standards/_registry.md`
   - the active registered project-local standard files in scope
6. `tooling_execution_contract`
   - `tooling_execution_policy.md`
   - `specflow/tooling/README.md`
   - `specflow/tooling/cmd/**/*.go`
   - `specflow/tooling/internal/**/*.go`

`ProjectSpec` and `flow` do not become independent review blocks.
They are reviewed inside `review_and_command_core`, plus their process and tooling contracts in the other fixed blocks.

## 4. Required Cross-Block Convergence Checks

For the default governance-baseline review, the minimum cross-block convergence checks are:

1. `review_and_command_core <-> shared_governance`
2. `review_and_command_core <-> impact_reconciliation`
3. `review_and_command_core <-> process_and_state_contracts`
4. `review_and_command_core <-> entry_and_project_local_extension`
5. `review_and_command_core <-> tooling_execution_contract`
6. `shared_governance <-> impact_reconciliation`
7. `shared_governance <-> tooling_execution_contract`
8. `process_and_state_contracts <-> impact_reconciliation`
9. `process_and_state_contracts <-> tooling_execution_contract`
10. `entry_and_project_local_extension <-> tooling_execution_contract`

If a narrowed review still crosses one of those boundaries and the owner block is not included, the review must stop without `pass`.

## 5. Preconditions

Before execution:

1. make the review scope explicit
2. build one execution-local `review_plan`
3. map in-scope files into fixed review blocks
4. name the required cross-block convergence checks before final conclusions
5. if the scope is the default governance baseline, explicitly confirm:
   - shared-governance coverage
   - impact-reconciliation coverage
   - tooling coverage
6. if project-local governance standards are registered, resolve the active in-scope entries from `docs/project_standards/_registry.md`

If any in-scope file cannot be assigned to a review block, do not issue `pass`.

## 6. Procedure

1. collect the in-scope governance files
2. build the `review_plan`
3. review each fixed block for:
   - closure
   - side effects
   - contract drift
   - missing ownership
4. complete the required cross-block convergence checks
5. produce findings ordered by governance risk
   - every real finding must use the fixed finding contract from Section 7.1
   - do not collapse a real finding into a one-line conclusion with no repair guidance
6. issue the final result only after block review and cross-block convergence are both complete

## 7. Output Contract

The output must report at least:

1. the review scope
2. the execution-local `review_plan`
3. the fixed review blocks used
4. the file coverage per block
5. the shared-governance coverage result
6. the impact-reconciliation coverage result
7. the tooling coverage result
8. the cross-block convergence results
9. the findings result:
   - explicit `none` when no real finding exists
   - otherwise every finding must satisfy Section 7.1
10. the final conclusion:
   - `pass`
   - `blocked`

If the output does not explicitly report Items 5 through 9, the review is not complete.

### 7.1 Finding Contract

When `spec_flow_review` reports a real finding, that finding must be written as one self-contained repairable unit.

The minimum required fields are:

1. `title`
   - one short problem label
2. `severity`
   - required when the finding is graded under `severity_policy.md`
3. `background`
   - the minimum repository or rule context needed to understand why this finding matters
4. `what happened`
   - the concrete mismatch, drift, omission, or conflict that was observed
5. `impact`
   - what governance risk, flow break, or downstream instability this creates
6. `recommended fix`
   - the concrete repair direction that should be executed next
7. `why this fix is the minimal correct fix`
   - why the recommendation closes the problem without inventing a wider redesign
8. `blocking`
   - explicit `yes` or `no`
9. `evidence`
   - the file refs, block boundary, or tool/runtime result that directly supports the finding

Additional rules:

1. if `severity` is present, the finding must satisfy the shared explanation baseline from `specflow/framework/docs/agent_guidelines/severity_policy.md`
2. `recommended fix` must be specific enough that a later user instruction such as "go fix it" can clearly refer back to that proposed repair without requiring a second clarification round
3. do not replace `recommended fix` with a vague statement such as "should be aligned" or "needs cleanup"
4. if more than one plausible repair exists and the review cannot justify one minimal correct fix, the finding must say that the repair path is still unresolved and the review must not present a guessed fix as settled
5. when no real finding exists, the output must say so explicitly instead of omitting the finding section

## 8. Non-Goals

This flow does not:

1. replace `shared_ops`
2. replace `impact_sync`
3. review business truth by default
4. treat recently touched governance files as the whole scope unless the user explicitly narrows it that way
5. prove design adequacy, human operability, or design worthiness

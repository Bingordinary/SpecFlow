# Spec Flow Review

## 1. Purpose

This flow reviews the Spec-driven governance mechanism itself together with the governance tooling implementation that executes fixed governance actions.

It does not review a business module's `stable`, `candidate`, or implementation design by default.

Plain user input `spec_flow_review` means the full default governance-baseline review defined in Section 3 unless the user explicitly narrows the scope.
The absence of words such as `shared_ops`, `shared governance`, `tooling`, or `script` does not narrow that default review.

It answers four questions:

1. whether the governance rules under review still keep the whole Spec Flow closed
2. whether those rules or the tooling layer introduce side effects into existing flows
3. whether the tooling layer still matches the documented governance contract instead of becoming a second semantic source of truth
4. if problems exist, what their severity, blocking status, and recommended repair actions are

Here, "Spec Flow" means the governance mechanism formed by these objects together:

1. `specflow/framework/docs/agent_guidelines/*.md`
2. `specflow/framework/docs/agent_guidelines/commands/*.md`
3. template-side governance baseline files under `specflow/templates/root/docs/specs/` where those files define the framework's default gate semantics
4. template entry-index files under `specflow/templates/root/` that define the framework-owned managed block content for supported hosts
5. `specflow/framework/docs/agent_guidelines/entry_index_registry.md` only where its rules affect project-side entry-file ownership or sync boundaries
6. `specflow/framework/docs/agent_guidelines/project_standards_policy.md` where project-local standards affect governance closure
7. the current project's registered project-local standards under `docs/project_standards/` because those rules may tighten or clarify governance decisions used by the executor
8. governance-tooling contract documents and governance-tooling source under `specflow/tooling/` where those files implement fixed governance actions defined by the framework baseline

This flow is not a module command and is not part of the module lifecycle managed by `docs/specs/_status.md`.

## 2. Review Goal

The goal is not "find as many issues as possible." The goal is "find only the issues that would make Spec Flow distorted, uncontrollable, semantically unstable, or split across conflicting document and tooling meanings."

In plain words:

1. if something is only inelegant but does not harm flow correctness, it is not the focus here
2. if a rule or tooling path makes executors unsure which file to read, which step to run, or where to fall back, that is a real target
3. if a rule and the tooling implementation silently drift apart, that is also a real target
4. if tooling starts deciding semantics that should stay in rules or runtime judgment, that is also a real target

## 3. Scope

By default this flow reviews whether the governance rule system and the governance tooling execution layer remain self-consistent.
It does not review whether business-module design is good.

The default scope is the repository's formal Spec Flow governance baseline:

1. `specflow/framework/docs/agent_guidelines/*.md`
2. `specflow/framework/docs/agent_guidelines/commands/*.md`
3. `specflow/templates/root/docs/specs/_status.md` only where its template-side governance role affects interpretation
4. `specflow/templates/root/docs/specs/_check_result/README.md`
5. `specflow/templates/root/docs/specs/_plans/README.md`
6. `specflow/templates/root/docs/specs/_verify_result/README.md`
7. template entry-index files:
   - `specflow/templates/root/AGENTS.md`
   - `specflow/templates/root/GEMINI.md`
   - `specflow/templates/root/CLAUDE.md`
8. `specflow/framework/docs/agent_guidelines/entry_index_registry.md` only where project-side entry ownership or sync rules affect governance closure
9. `specflow/templates/root/docs/project_standards/_registry.md` only where its template-side governance role affects interpretation
10. the installed project-side `docs/project_standards/_registry.md`
11. the active project-local standard files currently registered there, reviewed as governance inputs for governance conflict, ambiguity, or gate-semantic drift against the framework baseline
12. tooling contract and explanation documents:
   - `specflow/framework/docs/agent_guidelines/tooling_execution_policy.md`
   - `specflow/tooling/README.md`
   - `docs/specflow_go_tooling.md`
13. tooling source files that implement fixed governance actions:
   - `specflow/tooling/cmd/specflowctl/*.go`
   - `specflow/tooling/internal/**/*.go`

Project-local governance review extension contract:

1. `spec_flow_review` supports project-local `review_standard` entries only on the surface `governance_baseline_review`.
2. `governance_baseline_review` means a project-local governance-self-consistency overlay applied only after the framework-baseline review defined in this file has already been completed for the current review scope.
3. supported review scenarios on that surface are:
   - `default_governance_baseline`
   - `narrowed_governance_scope`
4. `default_governance_baseline` means plain `spec_flow_review` with the full default scope from this file.
5. `narrowed_governance_scope` means a user-explicitly narrowed governance review that still stays inside this flow.
6. consumption is optional and happens only when active registered entries match the current surface and review scenario.
7. `spec_flow_review` may consume only registered entries whose shape is all of:
   - `type=review_standard`
   - `surface=governance_baseline_review`
   - `consumed_by=spec_flow_review`
   - `effect=clarify` or `effect=tighten`
   - `applies_to=all_targets_on_surface` or `applies_to=review_scenario:<supported scenario name>`
8. consumed project-local standards may tighten or clarify only:
   - closure review
   - side-effect review
   - tooling-contract review
   - post-review handling review
   - structured findings
   - the final `pass | blocked` conclusion
9. consumed project-local standards must not:
   - widen the file scope beyond the current review scope
   - redefine severity meanings
   - redefine mandatory shared-governance coverage
   - redefine mandatory tooling coverage
   - create new result types
   - create project-side write-back requirements
10. this flow allows no project-side extension write-back container; project-local review results remain inside the normal review report only

Additional rules:

1. The template process READMEs are part of the default governance baseline because they directly affect the framework's default gate interpretation, even though they are not business truth files.
2. This flow does not automatically expand into all of `specflow/templates/root/docs/specs/**`.
3. Installed project files under `docs/specs/**` are not in the default scope unless the user explicitly narrows the review to project-instance governance.
4. Business-module `stable`, `candidate`, and process-instance files are not in the default scope.
5. The default entry-index set for this flow is the template entry set under `specflow/templates/root/`, not executor guesswork and not the project-side registered-file set.
6. `entry_index_registry.md` may still be read in this flow, but only to check whether project-side entry ownership and sync rules remain coherent with the template-side design.
7. The default governance baseline explicitly includes shared-governance rule files under `specflow/framework/docs/agent_guidelines/`, at minimum `shared_ops.md`, `shared_new.md`, `shared_extract.md`, `shared_bind.md`, `shared_topology.md`, `shared_sync.md`, and `shared_escape.md`.
8. Content truth files consumed by governance rules may be read only to confirm how governance binds, reads, or constrains them. Their own business or engineering content is not reviewed by default here.
9. If `shared_topology` or `shared_sync` exists, this flow only reviews whether the defined shared flows together close the Shared Contract lifecycle. It does not replace their actual reconciliation work.
10. If project-local standards are part of the framework baseline extension surface, this flow reviews both:
   - whether their registration and consumption rules remain closed
   - whether the current project's registered project-local standard content introduces governance conflict, ambiguity, or gate-semantic drift against the framework baseline
11. Unregistered files under `docs/project_standards/` are not in the default review scope because they are not formal governance inputs.
12. The active project-local standard files in the default scope are governance-input review targets even when they are normally consumed by other commands such as `cand_check`.
13. The optional `governance_baseline_review` surface is narrower than the default governance-input scope:
   - it controls only which registered project-local `review_standard` entries may tighten or clarify the `spec_flow_review` result itself
   - it must not be used to narrow the governance-input read set defined by the default review scope
14. Compiled binaries under `specflow/tooling/bin/` are not in the default review scope because they are build artifacts rather than the default review target of governance truth and implementation contract.

Do not automatically reinterpret `spec_flow_review` as "review current git diff", "review files touched in this session", or "review recently changed governance files" unless the user explicitly narrows scope that way.

The review content is fixed into four classes:

### 3.1 Closure Review

Check whether the reviewed governance rules and tooling responsibilities still allow the flow to run from entry to stop point without orphaned responsibilities.

At minimum:

1. entry conditions are explicit
2. operated objects are explicit
3. responsibilities among truth files, process files, index files, and tooling paths are still clear
4. upstream prerequisites, downstream consumers, and fallback points are written clearly
5. no state is created without any consumer
6. no action is required without a clear responsible command, rule, or tooling path
7. no dual source of truth defines the same thing twice
8. shared-governance routing, closure, and stop responsibility are explicitly covered rather than left implicit under a wildcard scope
9. no tooling function exists without a rule-defined ownership reason and execution position

### 3.2 Side-Effect Review

Check whether the reviewed rules or tooling implementation break existing flows or make old rules unstable.

At minimum:

1. no conflict or overlap with existing command responsibilities
2. no accidental change in the relation among `Next Command`, gate files, and git rules
3. no new path that bypasses an old gate
4. no regression that turns a previously explicit boundary back into executor guesswork
5. no ambiguous command matching where one user request can hit multiple flows
6. no conflict or drift between shared-governance routing rules and the main command system, checkpoint rules, or `system_constraints_change_proposal` boundary
7. no tooling path silently changes governance semantics that the rules still describe differently

### 3.3 Tooling Contract Review

Check whether the tooling layer still matches `tooling_execution_policy.md` instead of becoming a second semantic source of truth.

At minimum:

1. each tooling function satisfies the necessity contract
2. each tooling function stays inside the allowed execution-action surface
3. tooling does not perform forbidden semantic judgment
4. tooling contract documents, explanation documents, and source describe the same responsibility boundary
5. compiled tooling freshness is enforced so the executed binary still matches the current tooling source input set

### 3.4 Post-Review Handling Review

Check whether executors know what to do after a problem is found.

At minimum:

1. issues are graded by severity
2. blocking levels are explicit
3. the background, trigger path, and impact scope are explicit
4. a minimal executable fix suggestion is given
5. the next step is explicit: repair rules first, repair tooling first, repair both, or record and continue

### 3.5 Mandatory Review Planning And Fixed Review Blocks

This flow must not jump directly from file collection to final conclusions.
It must first create one execution-local `review_plan`.

`review_plan` means a temporary review plan that exists only for the current `spec_flow_review` execution.
It is not durable repository truth and it must not be written back as a new governance file.

The minimum `review_plan` content is:

1. the current review scope
2. the review blocks that will be used
3. the exact files assigned to each block
4. the required cross-block convergence checks before any final `pass | blocked` conclusion

For the default governance baseline scope, the `review_plan` must use exactly these fixed review blocks:

1. `review_and_command_core`
   - `specflow/framework/docs/agent_guidelines/spec_flow_review.md`
   - `specflow/framework/docs/agent_guidelines/spec_policy.md`
   - `specflow/framework/docs/agent_guidelines/command_policy.md`
   - `specflow/framework/docs/agent_guidelines/implementation_change_policy.md`
   - `specflow/framework/docs/agent_guidelines/git_policy.md`
   - `specflow/framework/docs/agent_guidelines/severity_policy.md`
   - `specflow/framework/docs/agent_guidelines/checkpoint_protocol.md`
   - `specflow/framework/docs/agent_guidelines/commands/*.md`
2. `shared_governance`
   - `specflow/framework/docs/agent_guidelines/shared_ops.md`
   - `specflow/framework/docs/agent_guidelines/shared_new.md`
   - `specflow/framework/docs/agent_guidelines/shared_extract.md`
   - `specflow/framework/docs/agent_guidelines/shared_bind.md`
   - `specflow/framework/docs/agent_guidelines/shared_topology.md`
   - `specflow/framework/docs/agent_guidelines/shared_sync.md`
   - `specflow/framework/docs/agent_guidelines/shared_escape.md`
3. `process_and_state_contracts`
   - `specflow/framework/docs/agent_guidelines/candidate_handoff_contract.md`
   - `specflow/framework/docs/agent_guidelines/process_snapshot_contract.md`
   - `specflow/framework/docs/agent_guidelines/downgrade_policy.md`
   - `specflow/framework/docs/agent_guidelines/recovery_policy.md`
   - `specflow/templates/root/docs/specs/_status.md`
   - `specflow/templates/root/docs/specs/_check_result/README.md`
   - `specflow/templates/root/docs/specs/_plans/README.md`
   - `specflow/templates/root/docs/specs/_verify_result/README.md`
4. `entry_and_project_local_extension`
   - `specflow/framework/docs/agent_guidelines/entry_index_registry.md`
   - `specflow/framework/docs/agent_guidelines/project_standards_policy.md`
   - `specflow/framework/docs/agent_guidelines/project_standard_create.md`
   - `specflow/templates/root/AGENTS.md`
   - `specflow/templates/root/GEMINI.md`
   - `specflow/templates/root/CLAUDE.md`
   - `specflow/templates/root/docs/project_standards/_registry.md`
   - `docs/project_standards/_registry.md`
   - the active registered project-local standard files in the current review scope
5. `tooling_execution_contract`
   - `specflow/framework/docs/agent_guidelines/tooling_execution_policy.md`
   - `specflow/tooling/README.md`
   - `docs/specflow_go_tooling.md`
   - `specflow/tooling/cmd/specflowctl/*.go`
   - `specflow/tooling/internal/**/*.go`

For a user-explicitly narrowed governance review:

1. still create a `review_plan`
2. use the smallest block set that fully covers the narrowed scope
3. if the narrowed scope touches a boundary whose interface owner lives in another block, include that owner block or stop without `pass`
4. do not invent ad hoc default-scope block names; either reuse the fixed block names above or explicitly state why a smaller subset is sufficient

Fixed block-completion rule:

1. a block is not complete until its own closure review, side-effect review, tooling-contract review where applicable, and post-review handling review have all been finished
2. a completed block result is still local only and must not be treated as the final result of `spec_flow_review`

Fixed convergence rule:

1. the final result must be decided only after block review and cross-block convergence review are both complete
2. for the default governance baseline scope, the minimum required cross-block convergence checks are:
   - `review_and_command_core` <-> `shared_governance`
   - `review_and_command_core` <-> `process_and_state_contracts`
   - `review_and_command_core` <-> `entry_and_project_local_extension`
   - `shared_governance` <-> `process_and_state_contracts`
   - `review_and_command_core` <-> `tooling_execution_contract`
   - `shared_governance` <-> `tooling_execution_contract`
   - `process_and_state_contracts` <-> `tooling_execution_contract`
   - `entry_and_project_local_extension` <-> `tooling_execution_contract`
3. if a user-narrowed scope still crosses an uncovered block boundary, do not issue a `pass`

## 4. Preconditions

Before execution:

1. the scope must be explicit; if the user did not narrow it, use the full governance baseline from Section 3
2. build one execution-local `review_plan` before detailed findings begin
3. read every governance file inside the current review scope
4. read any upstream governance files directly referenced by those files
5. if the scope affects command progression or gate interpretation, also read `specflow/templates/root/docs/specs/_status.md`, but treat it only as a template-side state-index file unless the user explicitly asks for more
6. if the scope is not narrowed, also read the three template process-rule READMEs under `specflow/templates/root/docs/specs/`
7. if the task is governance review or may modify governance rules, tooling contract files, entry files, or process-rule READMEs, read `specflow/framework/docs/agent_guidelines/git_policy.md`
8. if the scope is not narrowed, read `specflow/framework/docs/agent_guidelines/entry_index_registry.md` and the three template entry-index files under `specflow/templates/root/`
9. if project-local standards affect the reviewed rules, also read:
   - `specflow/framework/docs/agent_guidelines/project_standards_policy.md`
   - `specflow/templates/root/docs/project_standards/_registry.md`
   - `docs/project_standards/_registry.md`
10. determine the current project-local review scenario:
   - `default_governance_baseline` for plain `spec_flow_review`
   - `narrowed_governance_scope` when the user explicitly narrowed the governance review scope
11. after reading `docs/project_standards/_registry.md`, resolve the active registered project-local standard files for governance-input review in the current project instance
12. read the files from that governance-input review set
13. from that already-resolved governance-input review set, resolve only the active entries that match:
   - `type=review_standard`
   - `surface=governance_baseline_review`
   - `consumed_by=spec_flow_review`
   - the current `applies_to` selector
14. if the repository claims the project-local standards extension surface but `docs/project_standards/_registry.md` is missing or invalid, report governance drift instead of silently treating that case as "no project-local standards"
15. if the scope is the default governance baseline, the `review_plan` must use the fixed review blocks from Section 3.5
16. if the scope is the default governance baseline, explicitly confirm that the shared-governance rule set has been read, at minimum `shared_ops.md`, `shared_new.md`, `shared_extract.md`, `shared_bind.md`, `shared_topology.md`, `shared_sync.md`, and `shared_escape.md`
17. if the scope is the default governance baseline, explicitly confirm that the tooling execution contract set has been read, at minimum `tooling_execution_policy.md`, `specflow/tooling/README.md`, `docs/specflow_go_tooling.md`, and the in-scope tooling source files
18. if the scope is the default governance baseline and compiled tooling binaries are part of normal execution in the repository, explicitly confirm that the freshness-gate contract is covered at rule, source, and execution-entry level
19. do not treat reading only `command_policy.md`, `commands/*.md`, or other main command-chain files as sufficient for a default-scope review when shared-governance rules or tooling-contract files were not also covered
20. if any in-scope file cannot be assigned to a review block, or any required cross-block convergence check cannot be named explicitly, do not issue a `pass`

If you cannot determine exactly which governance files are being reviewed, do not issue a `pass`.
If a default-scope review did not cover the shared-governance rule set, do not issue a `pass`.
If a default-scope review did not cover the tooling execution contract set, do not issue a `pass`.
If the review output does not explicitly report the shared-governance coverage and result, do not issue a `pass`.
If the review output does not explicitly report the tooling coverage and result, do not issue a `pass`.
If the review output does not explicitly report the `review_plan`, block coverage, and required cross-block convergence result, do not issue a `pass`.

## 5. Procedure

1. locate the governance files inside the current review scope
2. if project-local standards are claimed, resolve the active project-local governance-input review set from `docs/project_standards/_registry.md` instead of scanning `docs/project_standards/` blindly
3. determine the current project-local review scenario:
   - `default_governance_baseline` for plain `spec_flow_review`
   - `narrowed_governance_scope` for a user-explicitly narrowed governance review
4. build the execution-local `review_plan`:
   - map the in-scope files into review blocks
   - for the default governance baseline scope, use exactly the fixed review blocks from Section 3.5
   - for a user-narrowed scope, use the smallest sufficient block set and explicitly list the required cross-block convergence checks
5. review the active project-local governance-input review set for governance conflict, ambiguity, or gate-semantic drift against the framework baseline
6. resolve which already-read governance-input entries also match the current `governance_baseline_review` overlay scenario
7. review each block from the `review_plan`
8. for each block:
   - map each rule point to the rule objects it affects
   - run the block-local closure review
   - run the block-local side-effect review
   - run the block-local tooling-contract review where applicable
   - run the block-local post-review handling review
   - mark the block complete only after all required block-local review classes are finished
9. enumerate the shared-governance rule files actually covered by the current review
10. explicitly review whether shared-governance routing, closure, boundary, and stop/checkpoint rules remain coherent with the main command system
11. enumerate the tooling contract files and tooling source files actually covered by the current review
12. explicitly review whether tooling necessity, allowed action surface, forbidden semantic judgment, and document/source consistency remain coherent with the framework baseline
13. explicitly review whether compiled tooling freshness is enforced by the combined behavior of `build-release`, runtime startup checks, and `doctor`
14. run the required cross-block convergence checks from the `review_plan`
15. after both block review and cross-block convergence review are complete, merge any matching project-local `review_standard` entries only as `tighten` or `clarify` input into structured findings and the final `pass | blocked` conclusion
16. grade every real problem by severity and blocking status
17. add background, trigger mechanism, impact scope, and repair suggestion to each finding
18. give an overall conclusion and the next action for the current review scope
19. do not issue a final `pass` unless every planned block and every planned cross-block convergence check has completed

Severity must use the shared meanings defined in:

1. `specflow/framework/docs/agent_guidelines/severity_policy.md`

Fixed principle:

1. judge whether there is a real problem first
2. judge how severe it is second
3. do not start with personal preferences and then retroactively call them problems

## 6. Review Boundary

### 6.1 Allowed Findings

Findings are allowed only if they hit at least one of these:

1. broken closure
2. incompatible rule conflict
3. harmful side effect
4. high ambiguity
5. gate-semantic drift
6. missing default-scope coverage of required shared-governance rule files
7. missing default-scope coverage of required tooling-contract files or tooling source files
8. missing coverage of a required review block
9. missing or incomplete required cross-block convergence review
10. tooling function without necessity justification under `tooling_execution_policy.md`
11. tooling semantic judgment where only execution work is allowed
12. tooling document/source drift that changes governance meaning
13. compiled tooling freshness missing, bypassed too broadly, or inconsistent with the documented contract

### 6.2 Findings That Should Not Be Reported By Default

Do not report the following by default:

1. wording preference only
2. naming-style preference only
3. personal taste about section organization
4. speculative suggestions without side-effect evidence
5. overdesigned suggestions that add rule complexity without clear risk reduction
6. subjective nitpicks that cannot be attributed to closure, conflict, side effect, ambiguity, or tooling-contract drift

## 7. Output Contract

The output should include:

1. review scope
2. the execution-local `review_plan`, including:
   - the review blocks used
   - the files assigned to each block
   - the required cross-block convergence checks
3. the exact governance files reviewed, or a stable grouped list that still makes file coverage auditable
4. an explicit block-coverage section that states:
   - which planned review blocks were completed
   - which planned review blocks were blocked or incomplete
   - whether each completed block finished closure review, side-effect review, tooling-contract review where applicable, and post-review handling review
5. an explicit shared-governance coverage section that states:
   - whether `shared_ops.md`, `shared_new.md`, `shared_extract.md`, `shared_bind.md`, `shared_topology.md`, `shared_sync.md`, and `shared_escape.md` were reviewed
   - whether the shared-governance review result is pass, blocked, or has findings
   - whether the review stayed at governance-rule level rather than executing a concrete shared request instance
6. an explicit tooling coverage section that states:
   - which tooling contract documents and tooling source files were reviewed
   - whether tooling necessity review passed, was blocked, or has findings
   - whether tooling non-judgment review passed, was blocked, or has findings
   - whether tooling document/source consistency passed, was blocked, or has findings
   - whether tooling freshness-gate review passed, was blocked, or has findings
7. an explicit project-local governance-input coverage section that states:
   - which active registered project-local standard files were reviewed as governance inputs
   - whether any of them introduced governance conflict, ambiguity, or gate-semantic drift against the framework baseline
8. an explicit cross-block convergence section that states:
   - which required block-to-block interfaces were reviewed
   - whether each required interface review passed, was blocked, or has findings
   - whether any uncovered block boundary prevented a final `pass`
9. overall conclusion
10. findings ordered by severity and blocking priority
11. for each finding:
   - what the problem is
   - why it happens
   - what it impacts
   - the minimal recommended fix
12. whether the current review passes or is blocked
13. when a project-local review surface was consumed:
   - which `surface` matched
   - which registered project-local standard files were used
   - how they tightened or clarified findings or the final conclusion
14. the next action

## 8. Non-Goals

This flow does not:

1. review business-module behavior design
2. verify implementation alignment for a concrete module
3. replace `cand_check`, `cand_verify`, or `stable_verify`
4. execute reconciliation work in place of `shared_sync`
5. treat unregistered files under `docs/project_standards/` as active governance inputs
6. treat platform binaries under `specflow/tooling/bin/` as the default governance review target
7. allow ad hoc default-scope review blocks

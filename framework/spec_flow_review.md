# Spec Flow Review

## 1. Purpose

`spec_flow_review` reviews the governance mechanism itself.

It answers five questions:

1. whether the governance rule set still closes the full Spec Flow
2. whether the tooling layer still matches the rule layer
3. whether rule-governance and impact-reconciliation semantics still converge with the command core
4. whether governance documents can make an executor operational without prior `specFlow` knowledge or avoidable reading cost
5. whether the repository may still claim one coherent governance baseline

Plain input `spec_flow_review` means the default governance-baseline review defined in this file unless the user explicitly narrows scope.

This flow does not review business truth by default.
It reviews the mechanism that governs business truth.
It does not prove that the current governance design is sensible, humane, or worth using as designed.
That judgment belongs to `spec_flow_design_review`.

## 2. Review Standard

`spec_flow_review` judges whether the in-scope governance rules are correct, closed, coherent, executable, and handoff-safe.

It does not pass a review only because the required files were read or the required slices were visited.
Each in-scope rule, file, slice, and cross-convergence path must satisfy the standards in this section.

The fixed standards are `content validity`, `logical closure`, `chain closure`, `state-space closure`, `governance closure and ownership`, `contract drift`, `cross-convergence`, `supporting truth lifecycle closure`, `agent operability`, `tooling boundary`, `project-instance compatibility`, and `project-instance migration closure`.

### 2.1 Content Validity

Governance content must be valid rule information.
It must not use wrong statements, unsupported claims, empty explanation, or text with no execution effect to create only apparent closure.

Content is valid only when:

1. each rule claim is supported by its owner file, template contract, tooling source, or another current in-scope governance rule
2. names, terms, states, command forms, field names, paths, inputs, and outputs really exist where the document says they exist
3. producer and consumer files accept the same names, states, field shapes, inputs, outputs, and result meanings
4. examples and explanatory text do not direct an executor toward an action that conflicts with the formal rule
5. each local rule changes at least one allowed action, forbidden action, stop condition, output, dependency order, writeback target, or resume path
6. each rule can execute against the current repository structure or explicitly stops when the required repository object is absent

If a statement is logically closed only because it rests on a wrong premise, wrong interface, wrong owner, wrong path, missing consumer, or impossible action, the affected slice must not be marked `passed`.

This review may judge whether governance content is wrong, unsupported, unowned, unconsumed, inconsistent with an actual interface, or not executable.
It must not judge whether the whole governance design is too heavy, humane enough, or worth maintaining as designed.
Those design-value judgments belong to `spec_flow_design_review`.

If a discovered concern is a design-value concern rather than a governance-correctness concern, report that boundary and point to `spec_flow_design_review`.
Do not use a design-value concern by itself as a `spec_flow_review` finding.

### 2.2 Logical Closure

Each in-scope governance file must be internally closed.

A file is internally closed only when a capable executor can determine from that file and its explicit links:

1. the entry condition for the rule or flow
2. the governing owner
3. what action is allowed
4. what action is forbidden
5. what must be read before action
6. where durable writeback or process-state writeback may happen
7. when execution must stop
8. what output or stop report is required
9. how execution resumes after a stop, checkpoint, repair, or downstream handoff

If one of those items is intentionally owned elsewhere, the file must link or name the owner clearly enough that the executor does not guess.

### 2.3 Chain Closure

When an in-scope file is one step in a governance chain, the review must include the in-scope files it calls, hands off to, consumes, or depends on.

A governance chain is closed only when:

1. the calling file and owner file describe the same governed object
2. outputs from one step are accepted by the next step with the same meaning
3. status values and result values are fixed and shared by every producer and consumer that uses them
4. permission boundaries match across the handoff
5. stop conditions have a legal resume owner or next action
6. downstream side effects are either closed in the same chain or explicitly handed to the correct owner
7. no step requires the executor to fill a missing interface, owner, state transition, or branch condition from memory or ordinary term meaning

If a chain is missing one required file, contains a conflicting interface, or only works by executor inference, the related local or cross-convergence slice must not be marked `passed`.

### 2.4 State-Space Closure

The review must prove that the governance mechanism has legal progress paths for important states.
Reading every relevant file is not enough.
A locally consistent rule is still invalid when one of its important results leaves the executor with no legal state-changing next action.

For this review, a `state` is any recorded or implied governance condition that can affect:

1. `Active Layer`
2. `Next Command`
3. truth writeback permission or truth writeback target
4. process-file presence, validity, freshness, cleanup, or reuse
5. command, review, migration, checkpoint, or tooling gate result
6. checkpoint position or resume permission
7. implementation permission
8. recovery owner

An `important state type` is any class of state that changes at least one of:

1. the next legal action
2. the allowed writeback location
3. the verification responsibility
4. the recovery path
5. the implementation permission boundary

A `transition` is the complete relation:

```text
current state + command/result/condition -> allowed writeback -> next state -> next legal owner/action
```

State-space closure requires all of the following:

1. every important command result, review result, migration result, checkpoint result, tooling freshness result, fallback result, blocked result, drift result, repair result, and impact-sync result in scope has a legal transition
2. the transition names the allowed writeback target, or explicitly states that no writeback is allowed
3. the transition names the next legal owner or action
4. the transition can change governance state, implementation state, or a documented blocker state before the same command, gate, or review is run again
5. a stop is closed only when the blocking condition, owner, allowed writeback, and resume action are explicit
6. process invalidation, stale fingerprints, changed truth, migration, impact sync, and recovery states identify the owner that may clear, rebuild, or replace the affected state

If `Next Command` remains the same command after a failed, blocked, stale, or fallback result, the review must prove that an intermediate legal action can change the state consumed by that command before rerun.
If no such action exists, or if the intermediate action is only implied by executor judgment, the review must report a dead-loop finding.

The review must not use "the files were read", "the local rule is self-consistent", or "an executor can decide what to do" as a substitute for transition proof.
Missing transitions, ownerless transitions, contradictory transitions, and same-command reruns without a legal state-changing source are findings.

### 2.5 Governance Closure And Ownership

Each in-scope owner area must close from a legal entry to one legal next action, final result, or required stop.

The review must find a real finding when an in-scope rule can cause:

1. ambiguous entry selection
2. missing truth, process-state, tooling, recovery, or close-out ownership
3. bypass of a required command gate, truth writeback gate, rule-governance gate, impact-reconciliation gate, recovery gate, or close-out gate
4. a side effect with no downstream owner
5. a branch that never rejoins a legal command, review, repair, or stop path
6. chat agreement, repository history, directory shape, or ordinary term meaning to substitute for durable governance truth

### 2.6 Contract Drift

Governance contracts must not drift across rule documents, templates, run-state files, tooling contracts, and tooling source.

Contract drift exists when two in-scope surfaces define or consume the same governance object differently, including:

1. different command names or entry forms
2. different state values or result values
3. different required fields or writeback containers
4. different path ownership rules
5. different lifecycle advancement or fallback meanings
6. different tooling responsibilities
7. different freshness, fingerprint, cleanup, sync, or validation rules

Any contract drift that can change execution, stop behavior, review judgment, or downstream ownership is a finding.

### 2.7 Cross-Convergence

Locally correct rules must still compose into one coherent governance baseline.

The review must test cross-convergence wherever one rule area depends on another rule area.
At minimum, cross-convergence covers routing, commands, project-instance migration, truth writeback, implementation gates, onboarding source decision, rule governance, impact reconciliation, process state, entry files, project-local standards, tooling, and recovery when those areas are in scope.

When onboarding source decision is in scope, the review must verify that candidate source fields, candidate main Spec text, evidence appendix handling, implementation permission, and lifecycle gates converge without allowing observed implementation behavior to become implementation truth outside the candidate main Spec.

If a narrowed review crosses a boundary whose owner slice is not included, the narrowed review must stop or explicitly remain non-baseline.
It must not claim default governance-baseline `pass`.

### 2.7.1 Supporting Truth Lifecycle Closure

Durable supporting truth must remain layer-correct across fork, promote, cleanup, rule release, and impact-reconciliation paths.

For this review, supporting truth includes:

1. stable and candidate main Spec files
2. stable and candidate appendix files
3. evidence appendix files
4. Rule refs and Rule files consumed by current-layer Specs
5. process snapshots and cleanup targets that preserve or delete those files
6. deterministic tooling that creates, retargets, deletes, validates, or reports those files

Supporting truth lifecycle closure requires all of the following:

1. a fork path that creates candidate truth from stable truth must prove whether every stable-layer supporting file referenced by the stable main Spec is copied, retargeted, intentionally omitted, or remains only historical evidence
2. a promote path that lands candidate truth as stable truth must prove whether every candidate-layer supporting file is migrated, absorbed, deleted as evidence, or intentionally left out of stable behavior truth
3. success cleanup must preserve current-round supporting truth until the owning command has migrated, absorbed, or intentionally deleted it
4. deterministic auto-fork tooling must implement the same supporting-file writeback, retarget, and cleanup contract as the command rule it mechanizes
5. current-layer main Specs must not depend on previous-layer appendix files as current behavior truth
6. stable main Specs must not point to candidate-layer appendix files as stable behavior truth
7. evidence appendix files must stay evidence only; they must not become stable behavior truth unless an in-scope command rule explicitly makes that migration legal

A review must not mark a supporting-truth lifecycle path `passed` only because files are present, readable, or shape-valid.
It must prove the current layer, link targets, cleanup ownership, and tool/source agreement for the path being reviewed.

If a fork, promote, auto-fork, cleanup, or impact-reconciliation path can leave a current-layer Spec dependent on the wrong layer's supporting truth, can delete current-layer supporting truth before it is absorbed or migrated, or can make tooling behavior diverge from command rules, the review must report a finding.

### 2.8 Agent Operability

Governance files must be operable by a capable executor without prior `specFlow` memory.

Default full-scope `spec_flow_review` must read and consume `specflow/framework/agent_operability_standard.md`.
A narrowed review must read and consume that standard whenever the narrowed scope includes entry behavior, routing, commands, project-instance migration, checkpoints, rule governance, process state, entry files, or tooling contracts.

Agent-operability review must cover execution clarity, content economy, and formal rule voice.
A pass claim for an in-scope governance file must not ignore an applicable agent-operability failure.

### 2.9 Tooling Boundary

Governance tooling may execute only mechanical work already decided by governance rules, prior human judgment, or explicit caller parameters.
Tooling must not become a second semantic source of truth.

Default full-scope `spec_flow_review` must read and consume `specflow/framework/tooling_execution_policy.md`.
A narrowed review must read and consume that policy whenever the narrowed scope includes governance tooling, tooling contracts, run-state tooling, tooling source, or document/source agreement for tooling.

The tooling review must verify tooling necessity, allowed mechanical action surface, forbidden semantic judgment, freshness rules, and agreement between tooling source and tooling-governing documents.

### 2.10 Project-Instance Compatibility

Default full-scope `spec_flow_review` must perform a narrow project-instance compatibility check for `docs/specs/`.

This check verifies only whether the current project's SpecFlow instance files can still be read and consumed by the current framework contracts, templates, commands, and tooling.
It does not review business truth correctness.

The compatibility check may judge only:

1. required file presence for current project-instance entry points
2. required section, table, field, frontmatter, status value, command name, reference, and binding shape
3. agreement between project-instance process files and the template-side process contracts
4. agreement between project-instance object references and `docs/specs/_status.md`, `docs/specs/repository_mapping.md`, and current framework path rules
5. whether existing project-instance files use names, states, command forms, and reference formats that the current framework can consume
6. candidate metadata shape for current candidates, including unit `candidate_intent`, unit `repair_basis` when required, `source_basis`, `evidence_appendix_ref`, required evidence appendix reference presence, and evidence appendix file shape when the current framework requires one
7. current-layer supporting-truth reference shape, including whether candidate main Specs avoid stable appendix dependencies and stable main Specs avoid candidate appendix dependencies
8. appendix frontmatter and path agreement for owner, layer, and file-prefix shape, without judging the appendix's business content

The compatibility check must not judge:

1. whether a unit, scenario, or rule describes the right business behavior
2. whether acceptance criteria are sufficient for the product
3. whether a candidate or stable Spec should make different design decisions
4. whether implementation actually satisfies a unit, scenario, or rule
5. whether the current governance design is worth using
6. whether an evidence appendix's observed behavior is business-correct or should be retained

If the project-instance compatibility check finds old file shape, unsupported status values, missing required references, invalid binding format, missing candidate source fields, invalid evidence appendix references, missing required evidence appendix files, or unreadable process-state shape, it is a `spec_flow_review` finding because the framework cannot safely operate on the current project instance.
If the compatibility check finds a current-layer main Spec whose supporting-truth references point to the wrong layer, or an appendix whose owner, layer, or path prefix disagrees with the current framework path rules, it is a `spec_flow_review` finding because current framework commands cannot safely consume that project instance.
If the discovered concern is only about the truth content being wrong, incomplete, or undesirable as business truth, report that it is outside this check and route it to the owning command, rule-governance flow, repository-mapping flow, or design review.

### 2.11 Project-Instance Migration Closure

Default full-scope `spec_flow_review` must review `spec_flow_migrate` as the owner of project-instance format migration after framework rule updates.

The migration closure check verifies only whether the migration flow can safely update old project-instance files to the current framework shape.
It does not review business truth correctness.

The migration closure check must judge:

1. exact entry routing for `spec_flow_migrate`
2. natural-language routing for requests to update old project-instance files to current framework contracts
3. migration read surface and target surface
4. mechanical writeback boundaries
5. forbidden compatibility aliases, fallback logic, and business-truth rewriting
6. process-state invalidation after migrated truth or support files change
7. registered entry managed-block handling
8. checkpoint and output contracts for blocked migration
9. agreement with tooling boundaries when existing tooling is used

If migration can rewrite project files without a current rule-derived target, preserve stale process pass claims, choose business meaning, or leave invalidated downstream state without a legal next action, it is a `spec_flow_review` finding.

### 2.12 Relationship To The Slice Catalog

The baseline slice catalog is an execution organization for this review.
It is not the review standard by itself.

Every local slice, cross-convergence slice, and dynamic slice must be judged against this section.
Coverage without the standards in this section is not sufficient for `pass`.

## 3. Default Scope

The default scope includes:

1. framework governance rules
   - `specflow/framework/*.md`
2. command rules
   - `specflow/framework/commands/*.md`
3. candidate intent standard rules
   - `specflow/framework/candidate_intents/*.md`
4. guidance skill rules
   - `specflow/framework/skills/*/SKILL.md`
5. template-side process and state contracts
   - `specflow/templates/docs/specs/_status.md`
   - `specflow/templates/docs/specs/_check_result/README.md`
   - `specflow/templates/docs/specs/_plans/README.md`
   - `specflow/templates/docs/specs/_plans/draft/README.md`
   - `specflow/templates/docs/specs/_plans/active/README.md`
   - `specflow/templates/docs/specs/_verify_result/README.md`
   - `specflow/templates/docs/specs/_governance_review/README.md`
6. template entry files
   - `specflow/templates/AGENTS.md`
   - `specflow/templates/GEMINI.md`
   - `specflow/templates/CLAUDE.md`
7. project entry files
   - `AGENTS.md`
   - `GEMINI.md`
   - `CLAUDE.md`
8. entry registry and project-standard governance files
   - `specflow/framework/entry_index_registry.md`
   - `specflow/framework/project_standards_policy.md`
   - `specflow/framework/project_standard_create.md`
   - `specflow/templates/docs/project_standards/_registry.md`
   - `docs/project_standards/_registry.md`
   - the active registered project-local standards in scope
9. tooling contract, tooling source input, and reader runtime input
   - `specflow/framework/tooling_execution_policy.md`
   - `specflow/tooling/README.md`
   - `specflow/tooling/cmd/**/*.go`
   - `specflow/tooling/internal/**/*.go`
   - `specflow/tooling/go.mod`
   - `specflow/tooling/manifest.tsv`
   - `specflow/tooling/go.sum` when it exists
   - `specflow/tooling/reader/web/**`

Default scope excludes project-instance truth and project-instance state files under `docs/specs/` from business-truth review.

Files excluded from business-truth review include:

1. `docs/specs/repository_mapping.md`
2. `docs/specs/_status.md`
3. `docs/specs/rules/stable/s_g_rule_repository_baseline.md`
4. `docs/specs/units/**`
5. `docs/specs/scenarios/**`
6. `docs/specs/rules/**`
7. `docs/specs/_check_result/**`
8. `docs/specs/_plans/**`
9. `docs/specs/_verify_result/**`
10. `docs/specs/_governance_review/**`

Those files may be reviewed for business-truth correctness only when the user explicitly narrows `spec_flow_review` to project-instance state, or when a command, repository-mapping flow, rule-governance flow, or verification flow consumes them under its own policy.

Default full-scope `spec_flow_review` must still perform the project-instance compatibility check from Section 2.10.
This check is narrow and does not turn `docs/specs/` into default business-truth review scope.

The compatibility input surface includes:

1. `docs/specs/_status.md`
2. `docs/specs/repository_mapping.md`
3. `docs/specs/rules/stable/s_g_rule_repository_baseline.md`
4. existing project process files under `docs/specs/_check_result/**`, `docs/specs/_plans/**`, and `docs/specs/_verify_result/**`
5. existing project truth files under `docs/specs/units/**`, `docs/specs/scenarios/**`, and `docs/specs/rules/**`, only for file shape, required fields, references, and binding format

`docs/specs/_governance_review/**` is not part of the compatibility input fingerprint.
The active full-scope run-state file is governed by the run-state procedure in Section 6, because including that file in its own slice fingerprint would create self-referential stale state.

Default scope must explicitly include:

1. the onboarding source decision rule set
   - at minimum `natural_language_routing.md` where it enters onboarding source decision or advance routing, `advance_policy.md`, `onboarding_decision_policy.md`, `spec_policy.md`, `implementation_change_policy.md`, `unit_new.md`, `unit_check.md`, `unit_plan.md`, `unit_impl.md`, `unit_promote.md`, `scenario_new.md`, `scenario_check.md`, `scenario_promote.md`, `candidate_handoff_contract.md`, `candidate_intent_policy.md`, and `candidate_intents/*.md`
2. the rule-governance rule set
   - at minimum `natural_language_routing.md` only where it defines the rule-governance branch, `rule_new.md`, `rule_extract.md`, `rule_bind.md`, `rule_topology.md`, `rule_sync.md`, and `rule_escape.md`
3. the guidance-skill rule set
   - at minimum `using-specflow-guidance/SKILL.md`, `project-framing/SKILL.md`, `scope-cutting/SKILL.md`, `solution-design/SKILL.md`, `design-quality-review/SKILL.md`, and `spec-writeback-guidance/SKILL.md`
4. the impact-reconciliation rule set
   - at minimum `impact_sync_policy.md`, `process_snapshot_contract.md`, `recovery_policy.md`, template `_status.md`, and the template-side process README files
5. the tooling execution contract set
   - at minimum `tooling_execution_policy.md`, `specflow/tooling/README.md`, the in-scope tooling source files, and the runtime reader web files
6. the agent-operability standard
   - at minimum `agent_operability_standard.md`, entry files, routing policy files, onboarding source decision files, command policy files, command files, candidate intent policy and standards, rule-governance files, guidance skill files, review policy files, and process-state contract files in the current review scope
7. the state-space closure check
   - at minimum routing policy, advance policy, command policy, command files, candidate intent policy and standards, implementation permission rules, process-state contracts, recovery rules, impact-sync rules, migration rules, and project-instance compatibility inputs needed to prove important non-success transitions
8. the project-instance compatibility check
   - at minimum project-instance status, repository mapping, global rules, existing process files, and existing formal truth files under `docs/specs/`, limited by Section 2.10
9. the project-instance migration flow
   - at minimum `spec_flow_migrate.md`, `natural_language_routing.md` where it routes project-instance migration, `command_policy.md` where it defines the non-command boundary, `checkpoint_protocol.md`, `process_snapshot_contract.md`, `recovery_policy.md`, `entry_index_registry.md`, and the template-side process and entry files that migration consumes
10. the supporting-truth lifecycle closure check
   - at minimum fork commands, promote commands, process cleanup and recovery rules, Rule sync and release-version paths, project-instance compatibility inputs, tooling contracts, and tooling source that creates, retargets, preserves, or deletes supporting truth

If any one of those ten coverage sets is missing from a default-scope review, that review is not complete and must not issue `pass`.

## 4. Baseline Slice Catalog

For the default governance-baseline review, the executor must use the baseline slice catalog below.

Baseline slices are the minimum review outline.
They do not limit what the review may discover.
If the review discovers a material risk that is not fully covered by a baseline slice, the executor must add a dynamic slice under Section 5.

The baseline slice catalog organizes review execution.
It does not replace the Review Standard in Section 2.

### 4.1 Local Baseline Slices

Local slices review one owner area for internal closure, side effects, contract drift, missing ownership, and local agent operability.

1. `scope_inventory`
   - verifies default-scope collection, excluded project-instance truth, active project-local standards, and unassigned file handling
   - includes the deterministic scope produced by `review collect-default-scope --flow spec_flow_review`
2. `review_entry_policy`
   - reviews `spec_flow_review.md`, `spec_flow_design_review.md`, `severity_policy.md`, and `checkpoint_protocol.md`
   - verifies review entry meaning, output contracts, finding contracts, and stop behavior
3. `routing_and_command_policy`
   - reviews `natural_language_routing.md`, `advance_policy.md`, `onboarding_decision_policy.md`, `command_policy.md`, `scenario_policy.md`, `spec_flow_migrate.md`, `candidate_intent_policy.md`, `candidate_intents/*.md`, `commands/*.md`, and `skills/*/SKILL.md`
   - verifies exact command routing, exact advance routing, exact project-instance migration routing, natural-language routing, onboarding source routing, unit command progression, scenario command progression, and guidance entry behavior
4. `truth_and_implementation_gates`
   - reviews `spec_policy.md`, `repository_mapping_policy.md`, `implementation_change_policy.md`, `onboarding_decision_policy.md`, `candidate_intent_policy.md`, `candidate_intents/*.md`, `candidate_handoff_contract.md`, `downgrade_policy.md`, and `recovery_policy.md`
   - verifies truth ownership, candidate source fields, evidence appendix ownership, implementation diversion, handoff, fallback, and recovery rules
5. `shared_governance`
   - reviews `natural_language_routing.md` only where it defines the rule-governance branch
   - reviews `rule_new.md`, `rule_extract.md`, `rule_bind.md`, `rule_topology.md`, `rule_sync.md`, and `rule_escape.md`
6. `process_and_impact_state`
   - reviews `impact_sync_policy.md`, `process_snapshot_contract.md`, `recovery_policy.md`, template `_status.md`, template `_check_result`, template `_plans`, template `_verify_result`, and template `_governance_review`
   - verifies process-state contracts, snapshot invalidation, impact handling, and governance-review run-state boundaries
7. `project_instance_contract_compatibility`
   - reviews the current project-instance files under `docs/specs/` only for format and contract compatibility with current framework rules
   - reviews `spec_flow_migrate.md` as the migration owner for old project-instance shape discovered by this slice
   - verifies status shape, repository mapping shape, global rules shape, process-file shape, formal object file shape, candidate source metadata shape, candidate intent standard shape, evidence appendix reference shape, evidence appendix file shape, current-layer supporting-truth reference shape, appendix owner/layer/path agreement, reference format, status values, command names, rule binding format, migration writeback boundary, migration state invalidation, migration checkpoint handling, and migration output closure
   - must not judge unit, scenario, rule, or evidence-appendix business truth correctness
8. `entry_and_project_extension`
   - reviews `entry_index_registry.md`, `project_standards_policy.md`, `project_standard_create.md`, registered entry files, template entry files, template project-standard registry, project registry, and active project-local standards in scope
9. `tooling_execution`
   - reviews `tooling_execution_policy.md`, `specflow/tooling/README.md`, in-scope tooling source files, and runtime reader web files
   - verifies tooling necessity, allowed mechanical action surface, forbidden semantic judgment, freshness, reader runtime coverage, and document/source/runtime agreement
10. `agent_operability_local`
   - reviews the agent-operability result recorded by each local slice against `agent_operability_standard.md`
   - verifies that local slice conclusions, including candidate intent policy and standard consumption, did not rely on prior conversation, ordinary term meanings, or avoidable repeated reading

### 4.2 Cross-Convergence Baseline Slices

Cross-convergence slices review whether locally correct rules still compose into one coherent governance baseline.

1. `routing_to_command_convergence`
   - verifies natural-language routing, exact command routing, exact advance routing, exact project-instance migration routing, guidance entry, and review entry behavior converge without ambiguous owner selection
2. `command_to_process_state_convergence`
   - verifies command pass, fallback, cleanup, snapshot, and process-file consumption rules converge
3. `truth_to_implementation_convergence`
   - verifies truth writeback, onboarding source decision, repository mapping, implementation gates, evidence appendix non-truth handling, handoff, and recovery converge
4. `state_space_closure`
   - depends on `routing_and_command_policy`, `truth_and_implementation_gates`, `process_and_impact_state`, and `project_instance_contract_compatibility`
   - verifies important advance loops, command results, fallback states, checkpoint states, drift states, blocked states, repair states, migration states, and impact-sync states have legal progress transitions
   - verifies same-command reruns have a legal state-changing source before the rerun
   - must not use file-read coverage or local rule consistency as a substitute for transition proof
5. `shared_to_impact_convergence`
   - verifies rule-governance changes correctly converge with impact reconciliation and downstream process-state invalidation
6. `entry_extension_to_review_convergence`
   - verifies entry files and project-local standards cannot bypass the framework baseline, narrow default scope silently, or change review meaning without owner rules
7. `tooling_to_rule_convergence`
   - verifies tooling executes only rule-decided mechanical work, does not become a second semantic source of truth, and does not introduce a migration command unless a rule owner defines its mechanical surface
8. `supporting_truth_lifecycle_convergence`
   - depends on `routing_and_command_policy`, `truth_and_implementation_gates`, `process_and_impact_state`, `project_instance_contract_compatibility`, and `tooling_execution`
   - verifies fork, promote, cleanup, rule `release-version`, rule sync, project-instance compatibility, and tooling-to-rule agreement for stable and candidate main Specs, appendices, evidence appendices, and Rule refs
   - must explicitly report the supporting-truth lifecycle paths walked; a generic cross-convergence `passed` statement is not sufficient
9. `project_instance_to_framework_convergence`
   - verifies the project-instance compatibility check and `spec_flow_migrate` compose with routing, command, process-state, repository-mapping, shared-binding, entry-file, and tooling rules without judging business truth content
10. `agent_operability_path_walk`
   - walks representative execution paths across routing, advance, command, shared, process-state, entry, and tooling rules
   - verifies a new executor can proceed from request to next legal action without hidden context

The final result must not issue `pass` until every required local baseline slice, every required cross-convergence baseline slice, and every dynamic slice is closed as `passed` or `skipped_not_in_scope`.

## 5. Dynamic Slices

Dynamic slices extend the baseline catalog during execution.
They are required when a discovered risk is not fully covered by an existing baseline slice.

Rules:

1. a dynamic slice may be local or cross-convergence
2. a cross-area risk must become a cross-convergence dynamic slice instead of being hidden inside one local slice
3. a dynamic slice may only increase review coverage; it must not weaken or replace a baseline slice
4. a dynamic slice must be added before final conclusion when the executor discovers:
   - a new dependency boundary
   - a new owner conflict
   - a new process-state or tooling interaction
   - a new agent-operability risk
   - a finding that needs a separate repairability check
5. every dynamic slice must record:
   - `slice_id`
   - `parent_slice_id`
   - `slice_type`
   - `review_question`
   - `why_added`
   - `input_files`
   - `depends_on`
   - `exit_condition`
   - `status`

## 6. Full-Scope Review Run State

Default full-scope `spec_flow_review` uses a run-state process file.

The process file is not a Spec, not durable behavior truth, and not a substitute for the review output.
It records review progress, slice inputs, stale status, findings, and resume position for one full-scope review run.

The run-state path is:

```text
docs/specs/_governance_review/spec_flow_review.md
```

`review_run_id` is a field inside the run-state file.
It must use this shape:

```text
YYYYMMDD-HHMMSS-{scope_label}
```

There must be at most one `spec_flow_review` run-state file in the repository at any time.
Starting a new full-scope default review must delete the previous `spec_flow_review` run-state file before writing the new run state.
The file name must not contain the run ID, because the run ID identifies the review round inside the file rather than creating a history archive.

### 6.1 When To Use Run State

Rules:

1. full-scope default `spec_flow_review` must use the run-state file procedure in this section
2. narrowed `spec_flow_review` does not use full-scope run state by default
3. a narrowed review may use a run-state file only when the user explicitly asks for resumable slice review
4. project-instance truth under `docs/specs/` remains outside default governance-baseline review even though the run-state file itself is read for resume handling

### 6.1.1 Run-State Tooling Boundary

Run-state files contain both mechanical fields and review judgment fields.

Mechanical fields must be written by deterministic tooling when the tooling is available.
If the tooling is unavailable, the executor must obtain UTC time from the runtime environment before writing timestamp fields.
The executor must not invent timestamps, input fingerprints, or stale-refresh results from conversation context.

The mechanical fields are:

1. `created_at`
2. `last_updated_at`
3. baseline slice skeleton rows
4. `input_fingerprint`
5. stale status changes caused only by changed or missing `input_files`

The deterministic tooling entry is `specflowctl review run-* --flow spec_flow_review`.

Rules:

1. `review run-init --flow spec_flow_review` creates, reuses, deletes, or recreates the fixed full-scope run-state file
2. `review run-validate --flow spec_flow_review` checks the run-state file shape and all fixed status values, including closed statuses; it is not a reuse decision
3. `review run-refresh --flow spec_flow_review` recomputes slice fingerprints and marks affected `passed` slices as `stale` only for an open run-state file
4. `review run-touch --flow spec_flow_review` updates only `last_updated_at` on a structurally valid run-state file
5. tooling must not decide whether a slice has passed review
6. tooling must not write finding content
7. tooling must not decide final `pass` or `blocked`

### 6.2 Startup Procedure

At the start of a full-scope review:

1. inspect `docs/specs/_governance_review/spec_flow_review.md`
2. if no unclosed run-state file exists, create a new run-state file and start at `scope_inventory`
3. if one unclosed run-state file exists, run the basic validity check from Section 6.3
4. if the basic validity check fails, delete the old run-state file, report the deletion reason, create a new run-state file, and start at `scope_inventory`
5. if the basic validity check passes, apply the timestamp rules from Section 6.4
6. if the existing file is in `closed_pass` or `closed_blocked`, delete it, report the deletion reason, create a new run-state file, and start at `scope_inventory`
7. the startup procedure must not scan a per-flow subdirectory or preserve old closed run-state files as review history

### 6.3 Basic Validity Check

The basic validity check verifies only that the run-state file can be used as an open progress file.
It does not judge whether old review conclusions are still semantically correct.
It is different from `review run-validate`, which validates file shape and fixed status values without deciding reuse.

For startup reuse, the file is open-valid only when:

1. the file can be read
2. `review_flow` is `spec_flow_review`
3. `scope_label` is `default_governance_baseline`
4. `status` is one of:
   - `in_progress`
   - `blocked_on_finding`
   - `ready_for_final`
5. all required run fields from Section 8.1 exist
6. `created_at` and `last_updated_at` use the timestamp format from Section 6.4
7. the baseline and dynamic slice tables can be parsed
8. every slice status is one of the fixed slice status values from Section 6.6
9. every baseline slice has `parent_slice_id` set to `none`
10. every dynamic slice has `parent_slice_id` set to an existing baseline or dynamic slice in the same run-state file

Rules:

1. `closed_pass` and `closed_blocked` are closed states and must not be reused
2. `review run-validate` may still report a closed run-state file as structurally valid when all required fields, tables, timestamps, and fixed status values are valid
3. any other run status value is invalid and fails the basic validity check
4. if `last_updated_at` cannot be parsed, the file fails the basic validity check

The basic validity check must not decide:

1. whether the old review plan still covers every current risk
2. whether old slice conclusions remain semantically trustworthy after framework changes
3. whether current `specFlow` design is still worthwhile

### 6.4 Timestamp Reuse Rules

The run-state file must update `last_updated_at` whenever a slice status, active slice, finding list, blocked reason, or resume step changes.

Timestamp format rules:

1. `created_at` and `last_updated_at` must use UTC ISO 8601 in this exact shape:
   - `YYYY-MM-DDTHH:MM:SSZ`
2. examples:
   - `2026-04-26T10:30:00Z`
3. timezone offsets other than `Z` are invalid
4. timestamps with missing seconds are invalid
5. invalid timestamps fail the basic validity check
6. `last_updated_at` later than the current UTC time fails the basic validity check

Reuse rules:

1. compute run-state age as current UTC time minus `last_updated_at`
2. if the run-state age is within 2 hours, automatically reuse the run-state file
3. if the run-state age is older than 2 hours and no older than 24 hours, ask the user whether to reuse the run-state file or delete it and start a new run
4. if the run-state age is older than 24 hours and no older than 7 days, ask the user, but recommend deleting the old run-state file and starting a new run
5. if the run-state age is older than 7 days, delete the old run-state file and create a new run unless the user explicitly requests continuing that exact run

When a user chooses to reuse an old run-state file, that choice accepts the old progress record as the continuation basis.
The executor still must refresh file fingerprints and stale statuses under Section 6.5.

### 6.5 Stale Slice Handling

Every slice must record `input_files` and `input_fingerprint`.

On reuse:

1. recompute each slice input fingerprint from the current files
2. change any `passed` slice with changed input fingerprint to `stale`
3. change any cross-convergence slice that depends on a stale slice to `stale`
4. keep unaffected slices in their current status
5. add dynamic slices for newly discovered risks

The authoritative refresh entry is `specflowctl review run-refresh --flow spec_flow_review`.
If that tooling entry is available, executors must use it to update `input_fingerprint` and stale slice state before resuming review work.
Manual fingerprint calculations may be used only as diagnostics; they must not be written into the run-state file or used to decide that a slice remains fresh.

### 6.5.1 Slice Input Fingerprint Contract

Slice input fingerprints use the same text normalization rules as `specflow/framework/process_snapshot_contract.md`.

For each file in `input_files`:

1. file paths must be repository-relative paths rendered with `/`
2. file paths must be sorted lexicographically before hashing
3. read the full file text
4. normalize the text using `process_snapshot_contract.md` Section 7
5. compute `sha256` of the normalized UTF-8 bytes
6. render the file hash as lowercase hexadecimal

The slice fingerprint is computed from the ordered file records.

For each sorted file, append these exact lines to the fingerprint payload:

```text
file_ref: <path>
file_sha256: <hex>

```

Then compute `sha256` of the full payload encoded as UTF-8 and render it as lowercase hexadecimal.

Rules:

1. an empty `input_files` list is invalid unless the slice status is `skipped_not_in_scope`
2. if any input file is missing during fingerprint refresh, the slice becomes `stale`
3. if a missing file prevents the slice from being reviewed, the slice must become `blocked`
4. executors must not use filesystem timestamps, file size, git metadata, or conversation history as fingerprint input
5. dynamic slices use the same fingerprint contract as baseline slices
6. executors must not use shell checksum output, editor display, temporary scripts, or conversation-derived hashes as the authoritative `input_fingerprint`

### 6.6 Run And Slice Status Values

Run status values are fixed:

1. `in_progress`
2. `blocked_on_finding`
3. `ready_for_final`
4. `closed_pass`
5. `closed_blocked`

Slice status values are fixed:

1. `pending`
2. `passed`
3. `blocked`
4. `stale`
5. `skipped_not_in_scope`

Blocked rules:

1. a blocking finding changes the run status to `blocked_on_finding`
2. while blocked, the next review action must be the repair path or re-review of affected slices
3. a blocked run must not advance to final conclusion until all blocking findings are resolved and affected slices are re-reviewed

## 7. Procedure

For full-scope review:

1. collect the default in-scope governance files
2. execute the run-state startup procedure from Section 6.2
3. build or refresh the baseline slice table
4. review local baseline slices
5. add required dynamic slices when new risks are discovered
6. review cross-convergence baseline slices
7. review any cross-convergence dynamic slices
8. refresh stale statuses whenever an input file changes during the run
9. produce findings ordered by governance risk
   - every real finding must use the fixed finding contract from Section 8.2
   - do not collapse a real finding into a one-line conclusion with no repair guidance
10. issue the final result only after all required baseline and dynamic slices are closed

For narrowed review:

1. make the narrowed scope explicit
2. map the narrowed scope to the relevant baseline slice or slices
3. add dynamic slices when the narrowed review discovers uncovered risks inside the narrowed scope
4. do not claim default governance-baseline `pass`

## 8. Output Contract

The output must report at least:

1. the review scope
2. whether full-scope run state was created, reused, deleted and recreated, or not used
3. the run-state file path when full-scope run state is used
4. the baseline slice table and slice statuses
5. the dynamic slice table and slice statuses, or explicit `none`
6. the stale slice result
7. the rule-governance coverage result
8. the guidance-skill coverage result
9. the impact-reconciliation coverage result
10. the tooling coverage result, including reader runtime coverage
11. the project-instance compatibility and migration-flow result
12. the agent-operability result, including local slice results and path-walk result
13. the supporting-truth lifecycle result:
   - fork paths reviewed for stable-to-candidate supporting truth retargeting
   - promote paths reviewed for candidate-to-stable supporting truth migration, absorption, or deletion
   - cleanup paths reviewed for preserving current-round supporting truth until owned handling completes
   - deterministic tooling paths reviewed for agreement with command rules
   - wrong-layer current reference cases found, or explicit `none`
14. the cross-convergence results
15. the state-space coverage result:
   - covered state carriers
   - covered commands and governance flows
   - key non-success transitions reviewed
   - same-command rerun cases and their legal state-changing source, or explicit `none`
   - uncovered important state types, if any, reported as findings
16. the findings result:
   - explicit `none` when no real finding exists
   - otherwise every finding must satisfy Section 8.2
   - when real findings exist, the final or stop report shown to the user must include every minimum required field from Section 8.2 for each finding
   - a run-state file may store the same finding fields, but pointing to that file does not satisfy the user-facing report requirement
   - do not summarize a real finding only as a problem statement, impact statement, or blocked reason
17. the final conclusion:
   - `pass`
   - `blocked`

If the output does not explicitly report Items 7 through 16, the review is not complete.

### 8.1 Run-State File Shape

The run-state file must contain these run fields:

1. `review_flow`
2. `review_run_id`
3. `scope_label`
4. `status`
5. `created_at`
6. `last_updated_at`
7. `active_slice`
8. `baseline_slice_table`
9. `dynamic_slice_table`
10. `finding_refs`
11. `blocked_reason`
12. `resume_next_step`

Each slice entry must contain:

1. `slice_id`
2. `slice_origin`
   - `baseline` or `dynamic`
3. `slice_type`
   - `local` or `cross_convergence`
4. `status`
5. `review_question`
6. `why_added`
   - use `baseline_catalog` for baseline slices
7. `parent_slice_id`
   - use `none` for baseline slices
   - dynamic slices must reference an existing baseline or dynamic slice
8. `input_files`
9. `input_fingerprint`
10. `depends_on`
11. `finding_refs`
12. `result_summary`
13. `exit_condition`
14. `resume_next_step`

### 8.2 Finding Contract

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

1. if `severity` is present, the finding must satisfy the shared explanation baseline from `specflow/framework/severity_policy.md`
2. `recommended fix` must be specific enough that a later user instruction such as "go fix it" can clearly refer back to that proposed repair without requiring a second clarification round
3. do not replace `recommended fix` with a vague statement such as "should be aligned" or "needs cleanup"
4. if more than one plausible repair exists and the review cannot justify one minimal correct fix, the finding must say that the repair path is still unresolved and the review must not present a guessed fix as settled
5. when no real finding exists, the output must say so explicitly instead of omitting the finding section

## 9. Non-Goals

This flow does not:

1. replace the rule-governance branch
2. replace `impact_sync`
3. review business truth by default
4. treat recently touched governance files as the whole scope unless the user explicitly narrows it that way
5. prove design adequacy, human operability, or design worthiness

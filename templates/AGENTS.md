## Host Instructions

If the current command or governance flow explicitly consumes project-local standards, follow only the registered files selected by `docs/project_standards/_registry.md` for the current `surface`, `consumed_by`, and `applies_to` scope.

Content outside the managed block below belongs to the host repository.

Keep repository-specific rules outside the managed block. `specFlow` tooling may update only the managed block.

## Mermaid Communication Rules

- When explaining a Mermaid diagram, refer to nodes by the exact visible text shown in the diagram. Do not refer to nodes only as hidden IDs such as `A`, `B`, or `C`.
- If short node identifiers are needed for repeated references, include the identifier in the visible node label as well, for example `B["B. Unit Main Spec"]`, and use that same visible name in the prose.

<!-- SPECFLOW:BEGIN -->
## specFlow Addendum

Use this entry procedure for requests that belong to `specFlow`.
Before any lifecycle action or file edit, choose the owning policy file and follow only that policy's allowed path.

### 1. First Read

1. If the request is an exact standard command, read `specflow/framework/command_policy.md`, then the matching file under `specflow/framework/commands/`.
2. If the request is exactly `spec_flow_review` or `spec_flow_design_review`, with or without a narrowing phrase, read the matching review policy directly.
3. If the request is exactly `spec_flow_migrate`, with or without a narrowing phrase, read `specflow/framework/spec_flow_migrate.md` directly.
4. If the request only asks for implementation-side edits and does not ask for truth, boundary, shared, system, scenario, governance, migration, or guidance work, read `specflow/framework/implementation_change_policy.md` first.
5. For every other `specFlow` request, read `specflow/framework/natural_language_routing.md` first.

After the first policy file routes the request, continue only through the routed policy, command, governance flow, or checkpoint path.

### 2. Pre-Action Rules

1. Do not edit implementation-side files until `specflow/framework/implementation_change_policy.md` proves the change is `implementation_only` or the routed command explicitly allows implementation.
2. Do not change behavior truth, acceptance truth, object ownership, rule truth, global rules, lifecycle state, or process files unless the active policy or command explicitly allows that write.
3. Resolve path ownership and object boundaries from `docs/specs/repository_mapping.md` when they matter; do not guess from directories.
4. Resolve existing `unit` or `scenario` state from `docs/specs/_status.md` before advancing any lifecycle step.
5. Read `docs/specs/rules/stable/s_g_rule_repository_baseline.md` when the request may affect repository-wide defaults, shared mechanisms, prohibitions, or explicit exceptions.
6. Enter rule-governance only through `specflow/framework/natural_language_routing.md`.
7. Keep registered entry index managed blocks consistent according to `specflow/framework/entry_index_registry.md`.

### 3. Terms That Must Not Be Guessed

These project terms must be interpreted only through the policy files:

1. `Spec`
2. `unit`
3. `scenario`
4. `stable`
5. `candidate`
6. `_status.md`
7. `repository_mapping.md`
8. `rule`
9. `checkpoint`
10. `implementation_change_policy.md`

### 4. Hard Stops

Stop instead of guessing when any of these are true:

1. the request's intent or target object is unclear
2. path ownership, object boundary, or support-surface ownership is unclear
3. a behavior, acceptance, boundary, shared, or system decision exists only in chat and has not been written into durable truth
4. implementation permission is not proven
5. rule-truth or global-rule ownership is unclear
6. a prerequisite command, truth writeback, checkpoint, or verification gate is required first
7. Spec, command, routing, implementation, checkpoint, or entry-sync rules conflict

### 5. Required Report

For any `specFlow` route, report the user-facing answer first and keep traceability details separate.

The user-facing answer must state the current state, next action, reason, expected result, and remaining gap in plain project-structure language when they apply.
It must not require the user to understand internal object-family names, command names, lifecycle state names, policy-file names, or governance-flow names.

The execution note may name the entry shape, first policy file, routed owner, files changed, next legal step, and stop reason.
It must not be required for the user to understand the answer.

### 6. Detailed Rule Owners

Detailed routing, object, command, checkpoint, implementation, migration, rule-governance, project-standard, and entry-sync rules live under `specflow/framework/`.
Project truth inputs live under `docs/specs/`.
<!-- SPECFLOW:END -->

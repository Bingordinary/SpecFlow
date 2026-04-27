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

### 1. First Actions For Every Request

Before editing files or advancing lifecycle state, do this:

1. Decide whether the request belongs to `specFlow`.
   - It belongs to `specFlow` when it asks for project design, implementation, verification, promotion, explanation, governance, repository mapping, shared truth, system constraints, or project-local standards.
   - If it is outside those areas, follow the host agent rules.
2. Classify the entry shape.
   - Exact standard command: read `specflow/framework/command_policy.md`, then read the matching command file under `specflow/framework/commands/`.
   - Exact governance review entry: if the request is `spec_flow_review` or `spec_flow_design_review`, with or without an explicit narrowing phrase, read the matching review policy directly.
   - Natural-language request: read `specflow/framework/natural_language_routing.md` before choosing any command, governance flow, or implementation step.
3. Continue from the first policy file you read.
   - For lifecycle, file, command, checkpoint, and output details, follow the routed policy files.

### 2. Natural-Language Request Procedure

When the request routes to `specflow/framework/natural_language_routing.md`, the executor must:

1. identify every intent fragment in the request, because one request may mix implementation, truth, mapping, shared truth, review, guidance, and explanation work
2. read only the current truth needed to route those fragments
3. resolve path ownership and object boundaries from `docs/specs/repository_mapping.md` instead of guessing from directories
4. resolve existing `unit` or `scenario` state from `docs/specs/_status.md` before choosing the next lifecycle step
5. read `docs/specs/system_constraints.md` when the request may affect repository-wide defaults or global exceptions
6. enter shared-governance only through the shared flow selected by `natural_language_routing.md`
7. enter guidance only when the request is not clear enough for formal truth writeback or a standard command
8. take only the smallest legal next step, or stop at the checkpoint required by the routing policy

### 3. Modification Gates

Before changing repo-tracked files, prove that the current route allows the change.

1. For implementation-side files, including code, tests, config, migrations, and build scripts, read `specflow/framework/implementation_change_policy.md` before editing.
2. If that policy returns `truth_writeback_required` or `boundary_unclear`, do not edit implementation files. Route to the required truth or boundary step first.
3. If a request includes both truth changes and implementation changes, handle truth first unless `implementation_change_policy.md` proves the implementation is already allowed by current truth.
4. If the change may create, modify, delete, stage, or commit Spec files, framework governance files, or registered entry index files, read `specflow/framework/git_policy.md` before git close-out.
5. Registered entry index files are `AGENTS.md`, `GEMINI.md`, and `CLAUDE.md`. Their managed blocks must stay consistent according to `specflow/framework/entry_index_registry.md`.

### 4. Terms That Must Not Be Guessed

These terms are project-specific. Use the policy files instead of ordinary software-engineering meanings.

1. `Spec`: a durable source-of-truth file, not a normal explanation document
2. `unit`: a governed object, not automatically a directory, package, service, or module name
3. `scenario`: an end-to-end trigger-to-outcome chain, not direct implementation ownership
4. `stable`: accepted truth
5. `candidate`: proposed truth for the current change round
6. `_status.md`: a state index, not behavior truth
7. `repository_mapping.md`: the truth for path ownership, object boundaries, and repository structure
8. `shared_contract`: shared truth reused across formal objects
9. `checkpoint`: a required stop report that records why execution cannot safely continue and how it can resume
10. `implementation_change_policy.md`: the mandatory gate before direct implementation-side modification

### 5. Hard Stops

Stop instead of guessing when any of these are true:

1. the request's intent or target object is unclear
2. path ownership, object boundary, or support-surface ownership is unclear
3. a behavior change has not been written into the required truth file
4. implementation permission is not proven by `implementation_change_policy.md`
5. shared-truth or system-constraint ownership is unclear
6. a prerequisite command or checkpoint is required before the requested work
7. Spec, command, routing, implementation, checkpoint, or git rules conflict
8. a decision exists only in chat and has not been written into durable truth

### 6. Required Report

For any request routed through `specFlow`, the executor's final or stop report must separate the user-facing answer from traceability details.
Project-structure language means the current repository's capability areas, delivery surfaces, entry points, and responsibility areas as proven by current repository truth or named by the user.

The user-facing answer must:

1. answer the user's goal first
2. use project-structure language before internal governance language
3. describe next actions as plain engineering actions
4. state the current state, next action, reason, expected result, and remaining gap when they apply
5. avoid requiring the user to understand internal object-family names, command names, lifecycle state names, policy-file names, or governance-flow names

The execution note, when needed, must appear after the user-facing answer and stay short.
It may state:

1. the entry shape and first policy file used
2. the current owner, command, governance flow, or checkpoint
3. the files changed, if any
4. the next legal step, if work remains
5. the reason execution stopped, if a checkpoint or clarification is required

The execution note must not be required for the user to understand the answer.

### 7. Detail Owners

Use these files for the detailed rules:

1. `specflow/framework/natural_language_routing.md`: natural-language intent closure, routing, missing intent, and multi-step request handling
2. `specflow/framework/command_policy.md`: standard command forms, command families, lifecycle order, and shared gate rules
3. `specflow/framework/spec_policy.md`: Spec objects, layers, source-of-truth boundaries, and file ownership rules
4. `specflow/framework/implementation_change_policy.md`: direct implementation classification and diversion rules
5. `specflow/framework/repository_mapping_policy.md`: repository structure and path-ownership governance
6. `specflow/framework/checkpoint_protocol.md`: checkpoint fields, stop reports, and resume behavior
7. `docs/specs/repository_mapping.md`: current project structure truth
8. `docs/specs/system_constraints.md`: current global technical baseline and system constraints
9. `specflow/framework/skills/using-specflow-guidance/SKILL.md`: guidance entry when a request is not ready for formal truth writeback
10. `specflow/framework/shared_new.md`, `shared_extract.md`, `shared_bind.md`, `shared_topology.md`, `shared_sync.md`, and `shared_escape.md`: internal shared-governance flows reached by natural-language routing
11. `specflow/framework/git_policy.md`: commit boundaries and git handling
12. `specflow/framework/entry_index_registry.md`: registered entry files and managed-block sync rules
<!-- SPECFLOW:END -->

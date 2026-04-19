# Spec-Driven Development Policy

## 1. Purpose

This file defines how formal module Specs and the global system-constraints Spec work in this repository.

It answers four questions:

1. which objects make up the formal module Spec system and the global system-constraints Spec
2. which files carry those objects
3. how an agent should read and use those files before implementation, review, or promotion
4. what kind of `candidate` Spec is sufficient to drive planning and implementation

This is a direct rule document for executors. It does not depend on unwritten conversation context.

---

## 2. Core Objects

### 2.1 Version Layers

Formal modules have two version layers by default:

1. `stable`
   - the currently effective formal truth for the module
2. `candidate`
   - the candidate truth prepared for the next version

These layers answer:

1. what the current formal baseline is
2. whether the current work is aligning the formal version or advancing the next version

### 2.2 System Constraints

`system_constraints` is the unique global system-constraint object.
It is not a normal module and does not enter `docs/specs/_status.md`.

It answers:

1. what the formally recognized technical baseline of the project is
2. which shared mechanisms the project should prefer to reuse
3. what default solution the project should prefer for certain engineering problems
4. which global practices are forbidden and which exceptions must be explicitly recorded

It does not answer:

1. how one module's internal state machine should be designed
2. how one module's implementation should be split into functions
3. what the full system topology must look like all at once

It has only one formally effective layer:

1. `docs/specs/system/stable/s_system_constraints.md`

It has no independent `_check_result`, `_plans`, or `_verify_result`.
It also has no independent candidate file.

Lifecycle rules:

1. it is not a command target; users should not execute `*:system_constraints`
2. when a module command needs the formal technical baseline, shared mechanisms, or global exceptions, and the file exists, it must read `docs/specs/system/stable/s_system_constraints.md`
3. modules may propose new global constraints only in their own `candidate`
4. `s_system_constraints.md` may be created or updated only during module `cand_promote:{module}`
5. therefore, `system_constraints` evolves through module command chains instead of an independent command chain

### 2.3 Module

`module_xxx` is the formal module name.
It is the only stable module identifier used by command entry and the status table. It is not the same as a file name.

### 2.4 Module Spec File

Each module has exactly one formal Spec file per version layer:

1. `stable` -> `s_{module}.md`
2. `candidate` -> `c_{module}.md`

A formal Spec file must cover at least:

1. module goal and boundary
2. key terminology
3. data structures and protocols
4. state machine and main flow
5. edge cases and error handling
6. verifiability and acceptance criteria

### 2.5 Formal Module vs Supporting File

Spec files in the repository are divided into four governance objects:

1. formal module files
2. single-module supporting expansion files
3. shared-contract files
4. `system_constraints`

Project-local standards under `docs/project_standards/` are not a fourth kind of module Spec object.
They are project-local governance inputs controlled by `specflow/framework/docs/agent_guidelines/project_standards_policy.md`.
They may constrain command execution only through the registered extension surface defined there.

#### 2.5.1 Formal Module Files

An object counts as a formal module only when all of the following hold:

1. it has a stable module name such as `module_xxx`
2. it is a legal target of `{command}:{module}`
3. it is registered as a formal module row in `docs/specs/_status.md`
4. it has its own formal Spec file for the relevant layer
5. that main Spec file itself satisfies the minimum content requirements instead of outsourcing core truth to supporting files for the long term

Being placed under `docs/specs/modules/candidate/` or having a module-like file name is not enough by itself.

#### 2.5.2 Single-Module Supporting Expansion Files

These include appendix files, topic expansions, Prompt source templates, extra examples, or expanded explanations of complex objects for one module.

Rules:

1. they support a formal module but do not enter the command chain independently
2. they must not enter `docs/specs/_status.md` independently
3. they must not produce their own `_check_result`, `_plans`, or `_verify_result`
4. once explicitly referenced by the main module body, they enter the truth-reading surface of that layer
5. executors must not read the main file while skipping an explicitly referenced supporting file

Directory rules:

1. candidate supporting files belong under `docs/specs/modules/candidate/appendix/` or an equivalent dedicated subdirectory
2. stable supporting files belong under `docs/specs/modules/stable/appendix/` or an equivalent dedicated subdirectory
3. the roots of `docs/specs/modules/candidate/` and `docs/specs/modules/stable/` should normally contain only main module files
4. if a supporting file remains in a root directory, that is directory drift
5. the first standard command that discovers that drift during mandatory checks must fix it before continuing

Frontmatter rules:

1. use `module: module_xxx`
2. use `layer: stable | candidate`
3. use `spec_version_ref: s_{module}@... | c_{module}@...`
4. do not use `id: module_xxx` because that is too easily misread as an independent formal module identifier

#### 2.5.3 Shared Contract Files

`shared_contract` files are not formal modules.
They are independent shared truth objects reused by multiple formal modules.

Examples include:

1. shared protocol text
2. shared output protocols
3. shared object-expansion text
4. shared failure semantics
5. shared few-shot or reuse-boundary descriptions

Rules:

1. they are not legal `{command}:{module}` targets
2. they may exist at both `candidate` and `stable`
3. for one `shared_contract_id`, at most one current `candidate` file and at most one current `stable` file may exist at the same time
4. candidate-layer and stable-layer files for the same `shared_contract_id` may coexist; the stable file is the current formal baseline and the candidate file is the current next-round draft
5. they enter a module's truth-reading surface only when explicitly bound in that module's current-layer `Global Constraint Alignment.shared_contract_refs`
6. stable-layer modules may bind only stable-layer Shared Contract files
7. candidate-layer modules may bind either stable-layer or candidate-layer Shared Contract files, but the bound layer must be explicit in `shared_contract_refs`
8. their lifecycle depends on the command chains of modules that bind them
9. `bound_modules` is only a declaration of which modules the current text is expected to serve; it does not replace formal binding semantics
10. they are not module appendices, because they do not belong to one module
11. they do not become `system_constraints` automatically during promotion

Shared-boundary rules:

1. `shared_contract` does not mean "might be reused later"; it means "one formal truth should exist independently because multiple formal modules depend on it now or that cross-module dependency is already architecturally explicit from the start"
2. the default path is still to keep the first appearance of content in the current module body or appendix
3. when a second formal module clearly needs the same formal truth, that truth should become a shared candidate instead of remaining duplicated in module-local truth
4. when the shared truth is already architecturally explicit from the start, a candidate-layer `shared_contract` may be created before any consumer module candidate exists
5. in that architecture-first case, the shared file is still valid candidate shared truth, but formal binding still begins only when a module current-layer `shared_contract_refs` points to it
6. if content is only thematically similar or structurally similar, it is not shared
7. use one shared object per shared file
8. do not permanently stuff unrelated shared topics into one umbrella shared file
9. use `specflow/framework/docs/agent_guidelines/shared_ops.md` for shared-governance routing and formal shared-boundary review

Boundary against `system_constraints`:

1. `shared_contract` answers "which shared truth multiple modules currently reuse"
2. `system_constraints` answers "which global default rules are formally effective for the whole project now"
3. a `shared_contract` may stay permanently independent even after promotion
4. only conclusions that have become project-wide default rules should be absorbed into `system_constraints`

Shared directory rules:

1. candidate shared files belong under `docs/specs/shared_contracts/candidate/`
2. stable shared files belong under `docs/specs/shared_contracts/stable/`
3. they must not remain under one module's appendix path pretending to be module-local appendix files
4. if such directory drift is found, the discovering standard command must migrate the file and fix bindings before continuing

Shared reading, invalidation, and cleanup rules:

1. if `shared_contract_refs` is not empty, executors must read the bound Shared Contract files together with the module's current-layer truth
2. `cand_check`, `cand_plan`, `cand_impl`, `cand_verify`, `stable_verify`, and `spec_fork` must not skip bound Shared Contract files
3. when both layers exist for one `shared_contract_id`, a module is affected only by the exact bound layer and file recorded in its current-layer `shared_contract_refs`, unless the current task also rewrites that module's binding
4. if a bound Shared Contract's effective truth changes, all module candidate-side process files still carrying the old snapshot become invalid and fall back to `cand_check`
5. a module promotion may generate or update a stable-layer Shared Contract while the candidate-layer Shared Contract for the same `shared_contract_id` remains in place for other candidate-layer modules
6. promoted stable modules must not keep binding candidate-layer Shared Contract files after promotion
7. if the current round cannot determine the post-promotion stable/candidate Shared Contract topology from repository truth, promotion or shared governance must stop instead of guessing
8. if the only delta is `bound_modules`, do not invalidate candidate-side process files on that basis alone; report governance drift instead
9. if a stable Shared Contract changes, any previously established claim that a module still aligns with `stable` must be re-read and re-judged
10. the exception to Rule 9 is a still-closing `cand_promote` round for the same module:
   - when that same promotion round wrote the module's new stable truth together with the module's current stable Shared Contract binding, treat that module's stable landing as owned by `cand_promote` rather than as a stale prior alignment claim for `shared_sync` invalidation
11. Shared Contract files are not cleaned up merely because one module finished promotion; they may be cleaned only when no module still binds them, when they are replaced by newer shared files, or when their stable conclusions have been fully absorbed into the formal global baseline
12. candidate-layer Shared Contract files must not be deleted merely because a stable-layer file for the same `shared_contract_id` was generated in one module's promotion; they may be deleted only after no candidate-layer module still binds them
13. when a command or shared flow changes bindings or topology so a touched Shared Contract file would have no formal bindings remaining, that same command or flow owns resolving the terminal state of that file in the same round instead of leaving orphaned shared truth for later guesswork
14. if no module still binds a touched Shared Contract file and the current round does not explicitly keep it as independently authored shared truth, the owner of the binding or topology change must delete that now-unbound file when Rules 11 and 12 allow that cleanup
15. if a touched Shared Contract file gains or regains one or more formal bindings in a later round, that same round must remove or stop carrying any `unbound_retention`, `unbound_retention_reason`, and `unbound_retention_owner` fields from the resulting bound file state
16. if `bound_modules` diverges from the real set implied by module `shared_contract_refs`, that is governance drift and must be repaired by the command responsible for the binding change
17. any task that changes `docs/specs/shared_contracts/**` or any module's `shared_contract_refs` must complete Shared Contract state reconciliation before claiming the state is closed

Shared frontmatter should include at least:

1. `shared_contract_id: shared_xxx`
2. `layer: stable | candidate`
3. `shared_version: <semver>`
4. `bound_modules`
5. `system_constraints_stable_ref`
6. `unbound_retention` when a touched now-unbound Shared Contract is intentionally kept
7. `unbound_retention_reason` when `unbound_retention` is present
8. `unbound_retention_owner` when `unbound_retention` is present

Additional rules:

1. if no module formally binds the shared truth yet, `bound_modules` may be `none`
2. expected future consumers should be recorded as body-level planning text rather than being treated as formal bindings before `shared_contract_refs` exists
3. `shared_version` must follow the Shared Contract semantic version rules defined in `specflow/framework/docs/agent_guidelines/git_policy.md`
4. if a touched Shared Contract file is intentionally kept after losing all formal bindings, the only durable writeback location for that decision is the Shared Contract file itself
5. that intentional-unbound keep result must be written in frontmatter as:
   - `unbound_retention: intentional`
   - `unbound_retention_reason: <why this file remains independent shared truth now>`
   - `unbound_retention_owner: <command_or_flow_name>`
6. chat output, command summaries, and temporary planning notes do not count as intentional-unbound writeback
7. do not use the intentional-unbound retention fields for architecture-first shared authoring that has not yet had any formal module binding; that case is governed by `bound_modules=none` plus body-level planning text
8. if a later round rebinds or deletes that file, remove or stop carrying the intentional-unbound retention fields in the resulting file state

### 2.6 What Counts As Touching Formal Behavior Truth

For direct implementation gating, a request touches formal behavior truth when it would create, remove, or change any formally acknowledged answer about:

1. module goal or module boundary
2. external protocols, field meanings, default values, validation rules, or error semantics
3. main flow, state transitions, or convergence semantics
4. acceptance criteria or other testable success conditions
5. Shared Contract body text or binding relations
6. project-wide default rules or explicit exceptions recorded through `system_constraints`

Rules:

1. implementation-only work such as pure refactors, tests, observability, performance optimization with unchanged semantics, or repairing an implementation deviation against already-explicit truth does not touch formal behavior truth
2. if repository truth is not explicit enough to tell whether a request would change formal behavior truth, do not guess from code; use `specflow/framework/docs/agent_guidelines/implementation_change_policy.md` and classify the request as `boundary_unclear`
3. if a request changes Shared Contract text, Shared Contract bindings, or the meaning of a `system_constraints` default rule or exception, it touches formal behavior truth even when the code diff itself looks local
4. if you are unsure whether a request touches formal behavior truth, treat it as touching formal behavior truth

---

## 3. `_status.md`

`docs/specs/_status.md` is the formal module status table.

It records only current state facts. It does not carry governance rules.

At minimum, each formal module row answers:

1. whether `stable` exists
2. whether `candidate` exists
3. which layer is active now
4. what the default next command is
5. any concise note explaining why that next command is the smallest actionable step

Executors must consume `_status.md` as the state index for module routing and fallback decisions.

---

## 4. Minimum Requirements For A Formal Module Spec

A formal module main Spec must be self-sufficient enough to drive downstream work.

At minimum it must define:

1. context and motivation
2. terminology
3. data structures or protocols
4. state machine or main business flow
5. edge cases and error handling
6. testability and acceptance criteria

It must not rely on unwritten chat context, author memory, or vague README vision as the real basis for behavior.

---

## 5. Candidate Adequacy

A `candidate` is sufficient for downstream planning and implementation only when both are true:

1. `progressability`
   - the candidate is clear enough to stably enter planning and implementation
2. `content completeness`
   - the key behavior truth that affects implementation results has been formally acknowledged in the candidate

For `cand_check`, missing items should be understood in three layers:

1. `critical`
   - if missing, implementation results may change or different executors may make different external behaviors
2. `important`
   - does not directly change results now, but meaningfully harms review stability, maintenance, or future closure
3. `elaboration`
   - affects readability or presentation only

If progressability fails or any critical completeness gap remains, the candidate must not pass.

If the missing blocker is user intent, boundary selection, or acceptance meaning rather than executor-side implementation detail, the command may pause through the checkpoint protocol instead of pretending the missing truth already exists.

Rules:

1. that pause is a structured checkpoint, not a new lifecycle stage
2. if the missing truth affects behavior, protocol, boundary, or acceptance semantics, the answer must be written back into candidate truth before the candidate may pass
3. chat-only clarification is never sufficient as the durable truth basis for downstream planning or implementation

---

## 6. Global Constraint Alignment

When a module involves technical choices, shared infrastructure, cross-module reuse, global exceptions, or system-level proposals, its current-layer Spec should explicitly include `Global Constraint Alignment` or an equivalent section.

At minimum that section should cover:

For both `stable` and `candidate`:

1. `system_constraints_stable_ref`
2. `shared_contract_refs`
3. `shared_mechanism_reuse_summary`
4. `global_constraint_exceptions`

For `candidate` only:

5. `system_constraints_change_proposal`

Rules:

1. if the formal global baseline exists, `system_constraints_stable_ref` must equal the current stable system-constraint version
2. if no formal global baseline exists yet, it must be `none`
3. if the module behavior depends on Shared Contract truth, `shared_contract_refs` must bind it explicitly using the Shared Contract binding contract from Section 6.1
4. if the module deviates from global constraints or Shared Contract truth, that deviation must be written explicitly instead of implied
5. `system_constraints_change_proposal` exists only in module `candidate`; it is not an independent command target or lifecycle object
6. a stable-layer Spec must not treat `system_constraints_change_proposal` as an active required field or active proposal container
7. `system_constraints_change_proposal` should state at minimum:
   - which global default rule is being added, changed, or removed
   - why the current formal baseline is insufficient
   - how the current module round implements and verifies against that proposal
   - which modules or shared contracts would be affected if promoted
8. if a stable-layer Spec explicitly records `system_constraints_stable_ref` and that recorded reference no longer matches the current formal global baseline state, the module may no longer claim it still aligns with `stable` and must fall back to `stable_verify`

### 6.1 Shared Contract Binding Contract

`shared_contract_refs` is the module-side formal binding source for Shared Contract truth.
It has only two legal forms:

1. literal `none`
2. a markdown list of Shared Contract binding items

Each binding item must be written as exactly:

1. `<shared_file_prefix>@<shared_version>`

Binding-item rules:

1. `<shared_file_prefix>` must be either `c_shared_xxx` or `s_shared_xxx`
2. `c_` means the bound file lives under `docs/specs/shared_contracts/candidate/`; `s_` means the bound file lives under `docs/specs/shared_contracts/stable/`
3. the exact `file_ref` is derived deterministically as `docs/specs/shared_contracts/<layer>/<shared_file_prefix>.md`
4. `<shared_version>` must equal the bound file frontmatter `shared_version`
5. after the derived file is read, its frontmatter `shared_contract_id` is the identity used for snapshotting, binding comparison, and affected-module derivation
6. if an item cannot be resolved to exactly one existing file with matching frontmatter version, the binding is invalid

Normalization rules:

1. use literal `none` only when the module current layer binds no Shared Contract files
2. do not use `null`, an empty list, omitted content, or natural-language placeholders for a non-empty binding set
3. duplicate binding items are forbidden
4. raw markdown list order is not semantically meaningful; consumers must normalize the binding set by derived `file_ref` before comparison
5. whenever a command or flow rewrites `shared_contract_refs`, it must write back that normalized order

Consumer rules:

1. any command or flow that derives a real binding set, affected-module set, or `shared_contract_snapshot` must first interpret `shared_contract_refs` through this contract
2. stable-layer modules may bind only `s_` items
3. candidate-layer modules may bind either `s_` or `c_` items, but the item itself must make the layer explicit
4. when sibling layers of the same `shared_contract_id` both exist, only the exact resolved item is effective for that module unless the current round also rewrites that module's binding

---

## 7. Process Files

Process files are not behavior truth. They are execution artifacts.

The main process files are:

1. `_check_result/{module}.md`
2. `_plans/{module}.md`
3. `_verify_result/{module}.md`

Their validity never depends on file existence alone.
They remain valid only when their binding fields still match the current candidate main file, the current-layer module appendix snapshot when applicable, the current global baseline state, and the current Shared Contract snapshot when applicable.

They must also satisfy the centralized candidate handoff contract defined in:

1. `specflow/framework/docs/agent_guidelines/candidate_handoff_contract.md`
2. `specflow/framework/docs/agent_guidelines/process_snapshot_contract.md`

When those bindings drift, the process file is invalid and the module must fall back to the smallest valid next command.

Additional rules:

1. process files are not checkpoints
2. process files must not be used as a substitute for writing updated truth back into candidate or appendix files
3. when a command reports fallback, blocking, or resume decisions about process-file invalidation, it should use the standardized `fallback_reason_code` first and only then add natural-language explanation
4. when a process file records `spec_fingerprint`, `module_appendix_snapshot`, `system_constraints_stable_fingerprint`, or `shared_contract_snapshot`, those fields must use the fixed definitions from `process_snapshot_contract.md`

---

## 8. Pre-Execution Self-Check

Before implementation, review, verification, or promotion work, executors must perform the mandatory pre-checks required by the active command or policy.

At minimum, the pre-check should catch:

1. state drift
2. directory drift
3. outdated process-file bindings
4. missing required upstream truth files
5. invalid command progression against `Next Command`

If those checks fail, do not continue as if the command were still valid.

---

## 9. Lifecycle Closure Rules

The default closure logic is:

1. `stable`-side verification keeps or restores confidence in the current formal version
2. `candidate`-side work must move through `cand_check -> cand_plan -> cand_impl -> cand_verify -> cand_promote`
3. invalid process files force fallback to the smallest still-valid step
4. Shared Contract changes may invalidate many modules at once and must be reconciled explicitly
5. formal promotion must clean the round's candidate files and process artifacts

Checkpoint relationship rules:

1. a checkpoint is a structured communication stop inside a command, not a second lifecycle
2. a checkpoint does not count as command success
3. when a checkpoint answer changes current truth, candidate or appendix writeback must happen before lifecycle resume

Do not invent a second lifecycle outside these rules.

---

## 10. Executor Discipline

Executors must follow these defaults:

1. do not bypass `docs/specs/` and guess behavior from code or memory
2. when uncertain whether something is a behavior change, treat it as one
3. do not let code go first on behavior changes
4. read only the files required by the current task, but do read all files that the current rule makes mandatory
5. do not silently narrow review scope or command scope without user authorization or an explicit rule
6. when a command blocks, falls back, or resumes with a standardized reason, report the `fallback_reason_code` before the free-form explanation
7. if clarification affects behavior truth, write it back to the current candidate or appendix instead of leaving it in chat

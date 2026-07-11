# Spec Flow Review

## 1. Purpose

`spec_flow_review` reviews the governance mechanism itself.
This file owns explicit `deep_audit` review for mechanism correctness.
Ordinary or plain exact `spec_flow_review` entry routes through `framework/governance/review.md` first and stays `scoped_review`.
The only full-scope mechanism review entry is exact `spec_flow_review:full`.

It answers five questions:

1. whether the framework documents are self-consistent and complete
2. whether the three commands (next, review, promote) cover the governance needs without gaps or overlap
3. whether the tooling boundary is correct — specflowctl does deterministic work, LLM does semantic judgment
4. whether an executor can operate the framework without prior specFlow knowledge
5. whether all framework files, templates, and governance files agree with each other

Deep audit must use exact `spec_flow_review:full`. Plain exact entry must not automatically start full-scope run-state review.

This flow does not review business truth by default.
It reviews the mechanism that governs business truth.
It does not prove that the current governance design is sensible, humane, or worth using as designed.
That judgment belongs to `spec_flow_design_review`.

### Terminology Note

"Review" in this file has two distinct meanings:

1. **`spec_flow_review`** — the meta-governance command that reviews the framework mechanism itself (Sections 1, 2, 6-9, and the file as a whole). This is a developer command, not part of the user workflow. See `framework/governance/review.md` for routing.

2. **`review`** — one of the three user workflow commands (next, review, promote) defined in Sections 2.3-2.4. This command reviews candidate spec quality and is implemented as `spec_validate`. It has no relation to `spec_flow_review`.

When reading Sections 2.3-2.4, "review" refers to the user workflow command. Everywhere else, "review" or `spec_flow_review` refers to the meta-governance command.

## 2. Review Standard

`spec_flow_review` judges whether the in-scope governance rules are correct, closed, coherent, executable, and handoff-safe.

It does not pass a review only because the required files were read or the required slices were visited.
Each in-scope rule, file, slice, and cross-convergence path must satisfy the standards in this section.

The fixed standards are `content validity`, `logical closure`, `process closure`, `command completeness`, `governance closure and ownership`, `contract drift`, `cross-convergence`, `supporting layer closure`, `agent operability`, `tooling boundary`, `project-instance compatibility`, `project-instance migration closure`, and `review scope completeness`.

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
9. how execution resumes after a stop, repair, or downstream handoff

If one of those items is intentionally owned elsewhere, the file must link or name the owner clearly enough that the executor does not guess.

### 2.3 Process Closure

The three commands (next, review, promote) form a simple process. The review must verify:

1. each command has a defined purpose and does not overlap with the others
2. `next` outputs enough information for the agent to start work
3. `review` produces a structured output (issue list) that the agent can act on
4. `promote` has a complete flow: review step → verify step → archive step
5. promote's archive step deterministically copies candidate files to stable directories
6. promote's review and verify steps are independent review sessions (subagent), not self-approval
7. if promote fails (review or verify finds issues), the outcome is clearly communicated and no files are archived

If a command is missing a required output, has undefined behavior for failure cases, or requires the executor to infer its purpose, the related slice must not be marked `passed`.

### 2.4 Command Completeness

The three commands must each have clearly defined boundaries. The review must verify:

1. **next**: given a unit name, outputs the unit's candidate and stable spec files, appendix files, rule references, and related units. Does NOT output process directives or "next step" instructions.

2. **review**: given a unit name, reviews the candidate spec quality. Uses a subagent session. Outputs a structured issue list with categories (format, cross-spec consistency, acceptance item clarity) — each category is PASS or FAIL with a reason. Does NOT block the agent from continuing work.

3. **promote**: given a unit name, runs three steps:
   a. Review phase: reads candidate spec(s), checks quality and cross-unit consistency
   b. Verify phase: for each acceptance item, checks implementation satisfies it
   c. Archive phase: copies candidate files to stable directories
   
   Promote must fail and report findings if either review or verify finds issues. Promote must not archive if any check fails.

Each command must have:
1. a defined input (what parameters it accepts)
2. defined output (what it returns or writes)
3. defined failure behavior (what happens on error)
4. no side effects outside its defined scope

Missing definitions or undefined behavior for any command is a finding.

### 2.5 Governance Closure And Ownership

Each in-scope owner area must close from a legal entry to one legal next action, final result, or required stop.

The review must find a real finding when an in-scope rule can cause:

1. ambiguous entry selection
2. missing truth, tooling, or process ownership
3. bypass of a required governance gate or impact-reconciliation gate
4. a side effect with no downstream owner
5. a branch that never rejoins a legal action or stop path
6. chat agreement, repository history, directory shape, or ordinary term meaning substituted for durable governance truth

### 2.6 Contract Drift

Governance contracts must not drift across rule documents, templates, run-state files, tooling contracts, and tooling source.

Contract drift exists when two in-scope surfaces define or consume the same governance object differently, including:

1. different command names or entry forms
2. different state values or result values
3. different required fields or writeback containers
4. different path ownership rules
5. different advancement or fallback meanings
6. different tooling responsibilities
7. different freshness, fingerprint, cleanup, sync, or validation rules

Any contract drift that can change execution, stop behavior, review judgment, or downstream ownership is a finding.

Delivery-specific path references in hook-injected content are not contract drift. Files such as `framework/concepts.md` are delivered to the agent session through platform hook injection at startup. Their path references use `installed_project`-format paths (e.g., `specflow/tooling/bin/`) because that is the delivery context expected by the hook system — the hook files are installed into a `specflow/`-prefixed directory tree. Reviewers must not flag delivery-context-specific paths in hook-injected content as drift. Governance files read directly from the filesystem must use `source_repo` paths.

### 2.7 Cross-Convergence

Locally correct rules must still compose into one coherent governance baseline.

The review must test cross-convergence wherever one rule area depends on another rule area.
At minimum, cross-convergence covers commands, project-instance migration, truth writeback, rule governance, impact reconciliation, hooks, concepts.md, and tooling when those areas are in scope.

If a narrowed review crosses a boundary whose owner slice is not included, the narrowed review must stop or explicitly remain non-baseline.
It must not claim default governance-baseline `pass`.

### 2.7.1 Supporting Layer Closure

Durable supporting truth must remain layer-correct across the promote path.

For this review, supporting truth includes:

1. stable and candidate main Spec files
2. stable and candidate appendix files
3. Rule refs and Rule files consumed by current-layer Specs

Layer closure requires:

1. promote must copy all candidate main Spec files and appendix files to the stable layer
2. stable layer must not depend on candidate layer files
3. candidate layer may reference stable layer files (baseline for comparison)

If promote can leave a candidate file unpromoted while its stable counterpart is overwritten, or can create a stable file that depends on candidate-layer content that no longer exists, the review must report a finding.

### 2.8 Agent Operability

Governance files must be operable by a capable executor without prior `specFlow` knowledge.

A narrowed review must include commands, project-instance migration, rule governance, hooks, concepts.md, and tooling contracts when the narrowed scope covers those areas.

Agent-operability review must cover execution clarity, content economy, formal rule voice, and self-containment under Section 2.12 — whether Agent-facing instruction files deliver essential phase instructions inline rather than through chain-linked reading.

A pass claim for an in-scope governance file must not ignore an applicable agent-operability failure.

### 2.8.1 Agent Bootstrap Injection Path Review

The agent receives SpecFlow governance content through platform-specific bootstrap injection at session startup, not through entry file reading. The injection chain and verification checklist are defined in `framework/hooks.md`.

A review that does not trace the injection path from the hooks to the agent's runtime context cannot claim that agent operability has been verified.

The reviewer must report, for every in-scope governance surface, which injection paths were checked, whether each check passed or produced a finding, and whether any platform is missing its required hook or plugin configuration. A pass claim for agent operability must not ignore an unreviewed or unresolved injection path.

### 2.8.2 Procedural Surface Minimization

> **Scope:** This section applies to `spec_flow_review:full` (deep audit) only.

LLM executors are associative approximators, not symbolic procedure executors. They match patterns in context rather than executing encoded procedures. Every independent procedural decision delegated to an LLM is an associative approximation and carries a probability of incorrect association.

The primary consequence for governance mechanism design is **surface minimization**: a procedural decision that can be resolved by deterministic rules (command completeness lookup, fixed mapping, closed-form check) must be resolved before reaching the executor. The first question for any governance instruction is not "is this instruction clear?" — it is "should this instruction reach the executor at all?"

This principle does not reduce executor autonomy for judgment decisions (quality evaluation, design choice, correctness assessment) — it applies only to procedural decisions (routing, permission boundaries, completion mechanics) that can be handled deterministically.

The reviewer must audit every governance step in scope for avoidable procedural decisions. For each governance step, the reviewer must enumerate every procedural decision that reaches the executor and classify it as one of:

1. **required_judgment** — the decision requires executor understanding, evaluation, or creativity and cannot be resolved deterministically.
2. **avoidable** — the decision can be resolved by state lookup, fixed mapping, or closed-form check, and must be moved to tooling.

The following conditions always violate this standard:

1. Routing decisions that affect the current step are embedded in prose conditional statements that the executor must match against natural language input, rather than being resolved by tooling from current state.
2. Write permissions for the current step are defined in a separate reference document that the executor must read and interpret, rather than being stated as part of the step's directive.
3. Completion syntax or valid outcomes must be inferred by the executor from general knowledge or from a framework document read separately, rather than being stated as part of the step's directive.
4. The executor must read two or more linked documents in sequence to assemble the procedural content (action, boundaries, completion) for a single step.

If any governance step has an `avoidable` procedural decision, the mechanism fails this standard. A failing review must report each avoidable decision as a finding and name the smallest correct repair — either moving the decision to tooling or stating it as part of a step-level directive that reaches the executor.

### 2.9 Tooling Boundary

Governance tooling may execute only mechanical work already decided by governance rules, prior human judgment, or explicit caller parameters.
Tooling must not become a second semantic source of truth.

Default full-scope `spec_flow_review` must read and consume `<framework-root>/tooling_execution_policy.md`.
A narrowed review must read and consume that policy whenever the narrowed scope includes governance tooling, tooling contracts, run-state tooling, tooling source, or document/source agreement for tooling.

The tooling review must verify tooling necessity, allowed mechanical action surface, forbidden semantic judgment, freshness rules, and agreement between tooling source and tooling-governing documents.

### 2.10 Project-Instance Compatibility

Default full-scope `spec_flow_review` must perform a narrow project-instance compatibility check for the source_repo project-instance surface.

That surface is template bootstrap files under `<template-root>/docs/specs/**` and does not require real project-instance `docs/specs/` files.

This check verifies only whether the current project's SpecFlow instance files can still be read and consumed by the current framework contracts, templates, commands, and tooling.
It does not review business truth correctness.

The compatibility check may judge only:

1. required file presence for current project-instance entry points
2. required section, table, field, frontmatter, and binding shape
3. agreement between project-instance object references and the layout-selected repository mapping file and current framework path rules
4. appendix frontmatter and path agreement for owner, layer, and file-prefix shape, without judging the appendix's business content

The compatibility check must not judge:

1. whether a unit or rule describes the right business behavior
2. whether acceptance criteria are sufficient for the product
3. whether a candidate or stable Spec should make different design decisions
4. whether implementation actually satisfies a unit or rule
5. whether the current governance design is worth using

If the project-instance compatibility check finds old file shape, missing required references, or invalid binding format, it is a `spec_flow_review` finding because the framework cannot safely operate on the current project instance.
If the compatibility check finds an appendix whose owner, layer, or path prefix disagrees with the current framework path rules, it is a `spec_flow_review` finding because current framework commands cannot safely consume that project instance.
If the discovered concern is only about the truth content being wrong, incomplete, or undesirable as business truth, report that it is outside this check and route it to the owning command, repository-mapping flow, or design review.

### 2.11 Project-Instance Migration Closure

Default full-scope `spec_flow_review` must review `spec_flow_update` as the owner of project-instance format migration after framework rule updates.

The migration closure check verifies only whether the migration flow can safely update old project-instance files to the current framework shape.
It does not review business truth correctness.

The migration closure check must judge:

1. entry routing for `spec_flow_update`
2. rejection of migration write authority for requests that do not explicitly invoke `spec_flow_update`
3. migration read surface and target surface
4. mechanical writeback boundaries
5. forbidden compatibility aliases, fallback logic, and business-truth rewriting
6. registered path handling
7. blocked-stop and output contracts for blocked migration
8. agreement with tooling boundaries when existing tooling is used

If migration can rewrite project files without a current rule-derived target, choose business meaning, or leave an invalidated downstream state without a legal next action, it is a `spec_flow_review` finding.

### 2.12 Self-Containment

When a governance file is an Agent-facing instruction file (operation policy that an executor reads directly to decide the next governed action), the file must be self-contained for its essential instructions.

A file fails self-containment when:

1. the file contains an instruction that the Agent must follow to complete the current phase, but the instruction body is only available by reading a linked file
2. the file requires the Agent to read N sequential linked files to obtain the set of essential phase instructions (chain reading)
3. the file uses a link as the primary delivery mechanism for a required action, allowed write, forbidden write, close condition, or gate requirement

Cross-file links are acceptable only for:

1. non-essential background context or design rationale
2. optional skill files that the Agent may choose to load
3. data references (file paths to specs, truth, evidence) that the Agent needs to read as input — these are not instructions about what to do

A review must find a self-containment finding when a governance file requires the Agent to follow a chain of two or more links to obtain essential phase instructions that should have been stated directly.

### 2.13 Tool-Enforcement Boundary

When a governance rule describes a hard constraint — an allowed write, forbidden write, required gate, or permission requirement that the executor must not violate — the review must judge whether the rule could be enforced by deterministic tooling (`specflowctl`).

Review rules:

1. if a hard constraint can be validated by a deterministic check (pattern match, state comparison, file existence, fingerprint comparison, phase check), the governance file must not rely on Agent self-enforcement alone; the rule must be implemented in tooling or a documented finding must explain why tooling enforcement is not feasible
2. if a hard constraint requires semantic judgment that cannot be mechanically validated, the governance file may state it as an Agent-facing rule, but the review must note this limitation
3. a governance file that lists multiple hard constraints without tooling enforcement for any of them is a finding — the design is relying entirely on Agent self-discipline

This standard does not require every rule to have tooling enforcement. It requires the review to distinguish between rules that could be enforced (and should be) and rules that inherently require judgment (and must rely on Agent capability).

### 2.14 Review Scope Completeness

The review must verify that its own scope definition is complete.

The review scope must be defined in terms of actual framework files, not abstract categories.
For each file listed in scope, the review must verify:

1. the file exists and is readable
2. the file's content is internally consistent
3. the file's content does not contradict other files in scope

If the scope definition can be interpreted in multiple ways, or if it includes abstract categories without concrete file lists, the review must report an ambiguity finding.

### 2.15 Atom System Integrity

When the governance framework uses an atom system (see `framework/_atoms/README.md`) to manage content that appears identically across multiple files, the review must verify atom integrity.

An atom system manages shared governance content through atom source files, a manifest, a deterministic generation script, and target files with `==ATOM_BEGIN:id==` / `==ATOM_END:id==` markers. The atom source file is the single canonical source for shared content; target files between markers are overwritten by generation.

Atom integrity requires all of the following:

1. **Manifest validity** — every row in `framework/_atoms/manifest.txt` must name an existing atom source file and a non-empty list of existing target files. Unreachable targets and dangling references are findings.

2. **Marker presence** — every target file listed in the manifest must contain the matching `==ATOM_BEGIN:id==` and `==ATOM_END:id==` markers. A target file missing its required markers is a finding.

3. **Content agreement** — the content between markers in every target file must match the atom source content (after deterministic normalization). Any divergence is a finding under Section 2.6 (Contract Drift). Running `./framework/_atoms/verify.sh` from the repo root is the authoritative check.

4. **Generation determinism** — running `./framework/_atoms/generate.sh` must produce output identical to the current target files (modulo atom-managed content). If generation changes any atom-managed content, that content is stale and the review must report it as a finding.

5. **Marker isolation** — atom marker lines (`==ATOM_BEGIN:*==` / `==ATOM_END:*==`) must appear on their own lines with no leading or trailing whitespace. Content outside atom markers must not be overwritten or altered by the generation script.

6. **No duplicate markers** — each atom_id may appear in a target file exactly once (one begin/end pair per atom per file). Duplicate markers for the same atom_id in the same target file are a finding.

7. **Committed atom consistency** — if the repository is a git repo, the atom source file and all its target files must be committed together when atom content changes. A commit that changes the atom source without updating target files (or vice versa) that would cause `verify.sh` to fail is a finding.

The review must check atom integrity whenever the in-scope governance surface includes files that participate in the atom system. A narrowed review must perform this check to the extent that the narrowed scope includes atom-managed files.

A pass claim for any slice that covers atom-managed content must not ignore an applicable atom integrity failure.

Atom system integrity failures are always contract drift findings (Section 2.6) because they represent divergence between the canonical source and its distributed targets.

### 2.16 Consumer-Aware Path Validation

When a deployable file (hook script, platform plugin, template bootstrap script) contains hardcoded paths that are resolved at runtime by a consumer, the reviewer must validate those paths from the consumer's execution context — not from the source layout in which the file was authored.

Platform hooks and plugins are installed into a `specflow/`-prefixed directory tree under the parent project (the `installed_project` layout), but authored under the `source_repo` layout where content lives at the repository root. The reviewer must verify that every hardcoded path in a deployable file resolves correctly in its deployment context.

Consumer-aware path validation requires all of the following:

1. **Identify the deployment context** — determine where the file is installed relative to the parent project root. Files deployed by `specflowctl` hooks installation (see `tooling/internal/install/install.go` `InstallHooks`) are placed into `specflow/hooks/` or the project-level `.opencode/plugins/` directory; their consumer sees the `installed_project` layout.

2. **Identify the root-resolution strategy** — determine how the file's runtime determines its base directory: self-relative navigation (`dirname $0` / `SCRIPT_DIR`), externally-provided runtime parameter (`PluginInput.directory`), environment variable (`CLAUDE_PLUGIN_ROOT`), or implicit working-directory convention. Each strategy produces a different base directory and requires different path validation.

3. **Validate every hardcoded path** — for each hardcoded file path in the deployable file, resolve it against the consumer's base directory (not the source layout) and verify the target file exists in the deployment layout. A path that does not exist at the consumer's runtime is a finding even if it exists in the source layout.

4. **Cover every platform variant** — when multiple platform plugins or hook scripts serve the same purpose (reading `framework/concepts.md` for context injection), each platform variant must be validated independently. A finding in one variant is not resolved by a correct path in another.

5. **Path-vs-content separation** — this standard applies to hardcoded paths in executable code (hook scripts, plugin JS files). It does not apply to path references in injected content text (the body of `framework/concepts.md`), which are governed by the delivery-specific exemption in Section 2.6. It does not apply to Go tooling that uses `specflowlayout.Resolve()` for layout-aware path construction.

A pass claim for agent-operability review must not ignore unresolved consumer-path findings in deployable files. Each unresolved path that would fail at runtime is a real finding under this standard.

## 3. Default Scope

This section applies only to explicit `deep_audit`.

Default scope uses the fixed `source_repo` layout.

- framework root: `framework/`
- template root: `templates/`
- tooling root: `tooling/`
- project-instance compatibility mode: template bootstrap compatibility under `templates/docs/specs/`

The default scope includes:

1. framework governance rules
   - `<framework-root>/*.md`
   - `<framework-root>/core/*.md`
   - `<framework-root>/governance/**/*.md` (recursive, includes subdirectories such as `rules/`)
   - `<framework-root>/operations/*.md`
2. atom system files (content governance infrastructure)
   - `<framework-root>/_atoms/README.md`
   - `<framework-root>/_atoms/manifest.txt`
   - `<framework-root>/_atoms/generate.sh`
   - `<framework-root>/_atoms/verify.sh`
   - `<framework-root>/_atoms/**/*.md` (recursive, all atom source files)
3. framework concept and command rules
   - `<framework-root>/concepts.md`
   - `<framework-root>/guidance/*/SKILL.md`
4. template-side project-instance bootstrap contracts
   - `<template-root>/docs/specs/repository_mapping.md`
   - `<template-root>/docs/specs/rules/stable/s_g_rule_repository_baseline.md`
5. tooling contract and tooling source
   - `<framework-root>/tooling_execution_policy.md`
   - `<tooling-root>/README.md`
   - `<tooling-root>/cmd/**/*.go`
   - `<tooling-root>/internal/**/*.go`
   - `<tooling-root>/go.mod`
   - `<tooling-root>/go.sum` when it exists
   - `<tooling-root>/scripts/**`

Default scope excludes project-instance truth files under `docs/specs/` from business-truth review.

Files excluded from business-truth review include:

1. `docs/specs/repository_mapping.md`
2. `docs/specs/rules/stable/s_g_rule_repository_baseline.md`
3. `docs/specs/units/**`
4. `docs/specs/rules/**`
5. `docs/specs/_governance_review/**`

Those files may be reviewed for business-truth correctness only when the user explicitly narrows `spec_flow_review` to project-instance state, or when a command, repository-mapping flow, or rule-governance flow consumes them under its own policy.

Default full-scope `spec_flow_review` must still perform the compatibility check from Section 2.10.
This check is narrow and does not turn `docs/specs/` into default business-truth review scope.

Compatibility input is template bootstrap compatibility under `<template-root>/docs/specs/**`.
It must not require real project-instance `docs/specs/repository_mapping.md` or project truth files.

`docs/specs/_governance_review/**` is not part of the compatibility input fingerprint.
The active full-scope run-state file is governed by the run-state procedure in Section 6, because including that file in its own slice fingerprint would create self-referential stale state.

Default scope must explicitly cover:

1. the concept and command rule set — at minimum `concepts.md` and `spec_writing_guide.md`
2. the rule-governance rule set — at minimum `rule_validate_checklist.md`, `rule_verify_checklist.md`, and `rule_promote_workflow.md`
3. the tooling execution contract set — at minimum `tooling_execution_policy.md`, `<tooling-root>/README.md`, and in-scope tooling source files
4. the agent-operability standard — at minimum `concepts.md` (hook-injected content), rule-governance files, and review policy files
5. the project-instance compatibility check — at minimum the layout-selected repository mapping, global rule, and formal truth compatibility inputs, limited by Section 2.10
6. the project-instance migration flow — at minimum `operations/migration.md`

If any one of those six coverage sets is missing from a default-scope review, that review is not complete and must not issue `pass`.

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
   - verifies default-scope collection, excluded project-instance truth, and unassigned file handling
   - includes the deterministic scope produced by `review collect-default-scope --flow spec_flow_review`
2. `review_entry_policy`
   - reviews `spec_flow_review.md`, `spec_flow_design_review.md`, `governance/review.md`, `governance/review_scope.md`, and `severity_policy.md`
   - verifies review entry meaning, output contracts, finding contracts, and stop behavior
3. `concept_and_command_policy`
   - reviews `concepts.md`, `operations/migration.md`, and `guidance/*/SKILL.md`
   - verifies the three commands (next, review, promote) have defined input, output, and failure behavior per Section 2.4
   - verifies project-instance migration routing and guidance entry behavior
4. `truth_and_implementation_gates`
   - reviews `spec_writing_guide.md`, `core/repository_mapping.md`, and `concepts.md`
   - verifies truth ownership and candidate entry rules
5. `shared_governance`
   - reviews `rule_validate_checklist.md`, `rule_verify_checklist.md`, `rule_promote_workflow.md`
   - verifies that rule validate/verify/promote checks cover design quality, consumer alignment, and post-promote cleanup
6. `process_and_impact_state`
   - reviews `governance/impact_sync.md`
   - verifies impact handling and governance-review run-state boundaries
7. `project_instance_contract_compatibility`
   - reviews the current project-instance files under `docs/specs/` only for format and contract compatibility with current framework rules
   - reviews `core/repository_mapping.md` and `spec_writing_guide.md` as the owner contracts for object family, reference format, and rule binding format
   - reviews `operations/migration.md` as the migration owner for project-instance shape drift discovered by this slice
   - verifies repository mapping shape, appendix owner/layer/path agreement, reference format, rule binding format, migration writeback boundary, migration blocked-stop handling, and migration output closure
   - must not judge unit, rule, or appendix business truth correctness
 8. `hook_check`
    - reviews hook configuration files: `specflow/hooks/hooks.json`
    - verifies `specflow/hooks/session-start` reads `framework/concepts.md` and produces correct platform-specific JSON output
    - verifies every platform plugin (`.opencode/plugins/specflow.js`, etc.) resolves its hardcoded file paths correctly from the deployment context per Section 2.16
    - verifies `framework/concepts.md` contains complete agent governance content (triggers, HARD RULES, commands reference)
    - reference: `framework/hooks.md` for the full verification checklist
 9. `tooling_execution`
   - reviews `tooling_execution_policy.md`, `<tooling-root>/README.md`, and in-scope tooling source files
   - verifies tooling necessity, allowed mechanical action surface, forbidden semantic judgment, freshness, and document/source agreement
10. `agent_operability_local`
    - reviews `concepts.md`, rule-governance files, review policy files, and Spec writing policy files in the current review scope
    - verifies that `concepts.md` (the hook-injected content) contains self-contained instructions: trigger phrases, HARD RULES, commands, and suggestion flow rules
    - verifies that local slice conclusions did not rely on prior conversation, ordinary term meanings, hidden layout assumptions, or avoidable repeated reading

### 4.2 Cross-Convergence Baseline Slices

Cross-convergence slices review whether locally correct rules still compose into one coherent governance baseline.

1. `command_to_process_convergence`
   - verifies the three commands (next, review, promote) converge with process closure rules from Section 2.3
2. `truth_to_implementation_convergence`
   - verifies truth writeback, repository mapping, implementation gates, and candidate entry rules converge
3. `shared_to_impact_convergence`
   - verifies rule-governance changes correctly converge with impact reconciliation
4. `hook_to_review_convergence`
   - verifies hook configuration and `concepts.md` injection cannot bypass the framework baseline, narrow default scope silently, or change review meaning without owner rules
5. `tooling_to_rule_convergence`
   - verifies tooling executes only rule-decided mechanical work and does not become a second semantic source of truth
6. `supporting_layer_convergence`
   - depends on `concept_and_command_policy`, `truth_and_implementation_gates`, `project_instance_contract_compatibility`, and `tooling_execution`
   - verifies promote path correctly migrates candidate supporting files to stable layer per Section 2.7.1
7. `project_instance_to_framework_convergence`
   - verifies the project-instance compatibility check and `spec_flow_update` compose with hook and tooling rules without judging business truth content
 8. `agent_operability_path_walk`
    - walks representative execution paths starting from the hook-injected content (`framework/concepts.md`), through triggers (`spec_validate`, `spec_verify`, `spec_promote`), commands, and tooling rules
    - verifies a new executor can proceed from the injected content to the correct first command without hidden context or prior `specFlow` knowledge
    - injection-to-command alignment under Section 2.8.1 must be explicitly reported for every walked path
    - verifies the injection chain for each supported platform per `framework/hooks.md`

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

1. exact `spec_flow_review:full` must use the run-state file procedure in this section
2. ordinary scoped `spec_flow_review` must use `framework/governance/review_scope.md` and must not use full-scope run state
3. project-instance truth under `docs/specs/` remains outside default governance-baseline review even though the run-state file itself is read for full-scope resume handling

### 6.1.1 Run-State Tooling Boundary

Run-state files contain both mechanical fields and review judgment fields.

Mechanical fields must be written by deterministic tooling when the tooling is available.
If the tooling is unavailable, the executor must obtain UTC time from the runtime environment before writing timestamp fields.
The executor must not invent timestamps, input fingerprints, or stale-refresh results from conversation context.

The mechanical fields are the fields allowed by this review's run-state contract:

1. `created_at`
2. `last_updated_at`
3. `review_layout`
4. baseline slice skeleton rows
5. `input_fingerprint`
6. stale status changes caused only by changed or missing `input_files`

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

For each file in `input_files`:

1. file paths must be repository-relative paths rendered with `/`
2. file paths must be sorted lexicographically before hashing
3. read the full file text
4. normalize the text (strip trailing whitespace from each line, replace all line endings with `\n`, remove trailing empty lines)
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

For ordinary scoped review, use `framework/governance/review_scope.md` instead of this full-scope slice procedure.
Ordinary scoped review must not use the full-scope run-state file, baseline slice table, dynamic slice table, or this final `pass | blocked` conclusion contract.

## 8. Output Contract

This output contract applies to explicit `deep_audit`.

The output must report at least:

1. the review scope
2. the review layout
3. the framework root, template root, tooling root, and project-instance compatibility mode
4. whether full-scope run state was created, reused, deleted and recreated, or not used
5. the run-state file path when full-scope run state is used
6. the baseline slice table and slice statuses
7. the dynamic slice table and slice statuses, or explicit `none`
8. the stale slice result
9. the rule-governance coverage result
10. the guidance-skill coverage result
11. the impact-reconciliation coverage result
12. the tooling coverage result, including reader runtime coverage
13. the project-instance compatibility and migration-flow result
14. the agent-operability result, including local slice results and path-walk result
15. the supporting layer result:
   - promote paths reviewed for candidate-to-stable supporting file migration
   - stable-layer dependency on candidate-layer files found, or explicit `none`
   - candidate-layer dependency on stable-layer files confirmed as baseline reference only
16. the cross-convergence results
17. the command completeness result:
   - next command input, output, and failure behavior verified
   - review command input, output, and failure behavior verified
   - promote command input, output, and failure behavior verified
   - undefined behavior or missing definitions found, or explicit `none`
18. the findings result:
   - explicit `none` when no real finding exists
   - otherwise every finding must satisfy Section 8.2
   - when real findings exist, the final or stop report shown to the user must include every required information item from Section 8.2 for each finding
   - a run-state file may store the same finding information, but pointing to that file does not satisfy the user-facing report requirement
   - do not summarize a real finding only as a problem statement, impact statement, or blocked reason
19. the final conclusion:
   - `pass`
   - `blocked`

If the output does not explicitly report Items 9 through 18, the review is not complete.

### 8.1 Run-State File Shape

The run-state file must contain these run fields:

1. `review_flow`
2. `review_layout`
3. `review_run_id`
4. `scope_label`
5. `status`
6. `created_at`
7. `last_updated_at`
8. `active_slice`
9. `baseline_slice_table`
10. `dynamic_slice_table`
11. `finding_refs`
12. `blocked_reason`
13. `resume_next_step`

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

### 8.2 Narrative Finding Contract

When `spec_flow_review` reports a real finding, that finding must be written as one self-contained repairable story.
The first paragraph must help a new maintainer understand the execution path that breaks.

The first paragraph must use plain language and must be 4 to 6 sentences.
It must answer all of these questions before detailed evidence or trace data appears:

1. who is executing the flow
2. what the executor is trying to complete
3. what the governing rule should make clear
4. where the actual rule, handoff, or state path loses direction
5. how the executor can take the wrong next step
6. what the smallest correct repair point is

Do not present a raw field dump as the user-facing finding.
Lists such as `background`, `what happened`, `impact`, and `recommended fix` may be used only as private drafting aids or trace details; they do not satisfy the user-facing finding requirement by themselves.

The first use of an internal term must explain the term in place.
For example, if a finding mentions a `command file`, it must state that this is the framework governance file that tells the executor what to read, what it may write, and when it must stop.

Every real finding must still contain these information items:

1. a title
   - one short problem label
2. severity
   - required for every real finding and must be one of `P0`, `P1`, `P2`, or `P3`
3. background
   - the minimum repository or rule context needed to understand why this finding matters
4. what happened
   - the concrete mismatch, drift, omission, or conflict that was observed
5. impact
   - what governance risk, flow break, or downstream instability this creates
6. recommended fix
   - the concrete repair direction that should be executed next
7. why this fix is the minimal correct fix
   - why the recommendation closes the problem without inventing a wider redesign
8. blocking
   - explicit `yes` or `no`
9. evidence
   - the file refs, block boundary, or tool/runtime result that directly supports the finding

Recommended user-facing shape:

```text
(Historical finding F-006 removed — entry files no longer contain routing logic. Agent bootstrap is handled by platform-specific hook injection.)
```

Additional rules:

1. `severity` must satisfy the shared explanation baseline from `<framework-root>/severity_policy.md`
2. severity describes harm level; it does not replace explicit `blocking`
3. `P0` and `P1` are normally blocking; `P2` and `P3` are normally non-blocking unless the finding explains why the current review must stop
4. `recommended fix` must be specific enough that a later user instruction such as "go fix it" can clearly refer back to that proposed repair without requiring a second clarification round
5. do not replace `recommended fix` with a vague statement such as "should be aligned" or "needs cleanup"
6. if more than one plausible repair exists and the review cannot justify one minimal correct fix, the finding must say that the repair path is still unresolved and the review must not present a guessed fix as settled
7. when no real finding exists, the output must say so explicitly instead of omitting the finding section

## 9. Non-Goals

This flow does not:

1. replace the rule-governance branch
2. replace `impact_sync`
3. review business truth by default
4. treat recently touched governance files as the whole scope unless the user explicitly narrows it that way
5. prove design adequacy, executor operability, or design worthiness
6. replace the default `scoped_review` front door in `framework/governance/review.md`

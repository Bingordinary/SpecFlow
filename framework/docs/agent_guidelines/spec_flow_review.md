# Spec Flow Review

## 1. Purpose

This flow reviews the Spec-driven governance mechanism itself, not a business module's `stable`, `candidate`, or implementation.

It answers three questions:

1. whether the governance rules under review still keep the whole Spec Flow closed
2. whether those rules introduce side effects into existing flows
3. if problems exist, what their severity, blocking status, and recommended repair actions are

Here, "Spec Flow" means the governance mechanism formed by these objects together:

1. `specflow/framework/docs/agent_guidelines/*.md`
2. `specflow/framework/docs/agent_guidelines/commands/*.md`
3. template-side governance baseline files under `specflow/templates/root/docs/specs/` where those files define the framework's default gate semantics
4. template entry-index files under `specflow/templates/root/` that define the framework-owned managed block content for supported hosts
5. `specflow/framework/docs/agent_guidelines/entry_index_registry.md` only where its rules affect project-side entry-file ownership or sync boundaries
6. `specflow/framework/docs/agent_guidelines/project_standards_policy.md` where project-local standards affect governance closure
7. the current project's registered project-local standards under `docs/project_standards/` because those rules may tighten or clarify governance decisions used by the executor

This flow is not a module command and is not part of the module lifecycle managed by `docs/specs/_status.md`.

## 2. Review Goal

The goal is not "find as many issues as possible." The goal is "find only the issues that would make Spec Flow distorted, uncontrollable, or semantically unstable."

In plain words:

1. if something is only inelegant but does not harm flow correctness, it is not the focus here
2. if a rule makes executors unsure which file to read, which step to run, or where to fall back, that is a real target
3. if a rule silently bypasses older rules or makes two rules fight each other, that is also a real target

## 3. Scope

By default this flow reviews only whether the rule system is self-consistent. It does not review whether business-module design is good.

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
11. only the project-local standard files currently registered there for governance-relevant consumption

Additional rules:

1. The template process READMEs are part of the default governance baseline because they directly affect the framework's default gate interpretation, even though they are not business truth files.
2. This flow does not automatically expand into all of `specflow/templates/root/docs/specs/**`.
3. Installed project files under `docs/specs/**` are not in the default scope unless the user explicitly narrows the review to project-instance governance.
4. Business-module `stable`, `candidate`, and process-instance files are not in the default scope.
5. The default entry-index set for this flow is the template entry set under `specflow/templates/root/`, not executor guesswork and not the project-side registered-file set.
6. `entry_index_registry.md` may still be read in this flow, but only to check whether project-side entry ownership and sync rules remain coherent with the template-side design.
7. Content truth files consumed by governance rules may be read only to confirm how governance binds, reads, or constrains them. Their own business or engineering content is not reviewed by default here.
8. If `shared_flow_reconcile` exists, this flow only reviews whether it closes the Shared Appendix lifecycle. It does not replace its actual reconciliation work.
9. If project-local standards are part of the framework baseline extension surface, this flow reviews both:
   - whether their registration and consumption rules remain closed
   - whether the current project's registered project-local standard content introduces governance conflict, ambiguity, or gate-semantic drift against the framework baseline
10. Unregistered files under `docs/project_standards/` are not in the default review scope because they are not formal governance inputs.

Do not automatically reinterpret `spec_flow_review` as "review current git diff", "review files touched in this session", or "review recently changed governance files" unless the user explicitly narrows scope that way.

The review content is fixed into three classes:

### 3.1 Closure Review

Check whether the reviewed governance rules still allow the flow to run from entry to stop point without orphaned responsibilities.

At minimum:

1. entry conditions are explicit
2. operated objects are explicit
3. responsibilities among truth files, process files, and index files are still clear
4. upstream prerequisites, downstream consumers, and fallback points are written clearly
5. no state is created without any consumer
6. no action is required without a clear responsible command or rule
7. no dual source of truth defines the same thing twice

### 3.2 Side-Effect Review

Check whether the reviewed rules break existing flows or make old rules unstable.

At minimum:

1. no conflict or overlap with existing command responsibilities
2. no accidental change in the relation among `Next Command`, gate files, and git rules
3. no new path that bypasses an old gate
4. no regression that turns a previously explicit boundary back into executor guesswork
5. no ambiguous command matching where one user request can hit multiple flows

### 3.3 Post-Review Handling Review

Check whether executors know what to do after a problem is found.

At minimum:

1. issues are graded by severity
2. blocking levels are explicit
3. the background, trigger path, and impact scope are explicit
4. a minimal executable fix suggestion is given
5. the next step is explicit: repair rules first and re-review, or record and continue

## 4. Preconditions

Before execution:

1. the scope must be explicit; if the user did not narrow it, use the full governance baseline from Section 3
2. read every governance file inside the current review scope
3. read any upstream governance files directly referenced by those files
4. if the scope affects command progression or gate interpretation, also read `specflow/templates/root/docs/specs/_status.md`, but treat it only as a template-side state-index file unless the user explicitly asks for more
5. if the scope is not narrowed, also read the three template process-rule READMEs under `specflow/templates/root/docs/specs/`
6. if the task is governance review or may modify governance rules, entry files, or process-rule READMEs, read `specflow/framework/docs/agent_guidelines/git_policy.md`
7. if the scope is not narrowed, read `specflow/framework/docs/agent_guidelines/entry_index_registry.md` and the three template entry-index files under `specflow/templates/root/`
8. if the scope is not narrowed and project-local standards affect the reviewed rules, also read:
   - `specflow/framework/docs/agent_guidelines/project_standards_policy.md`
   - `specflow/templates/root/docs/project_standards/_registry.md`
   - `docs/project_standards/_registry.md`
9. after reading `docs/project_standards/_registry.md`, read only the project-local standard files actively registered there and relevant to governance consumption
10. if the repository claims the project-local standards extension surface but `docs/project_standards/_registry.md` is missing or invalid, report governance drift instead of silently treating that case as "no project-local standards"

If you cannot determine exactly which governance files are being reviewed, do not issue a `pass`.

## 5. Procedure

1. locate the governance files inside the current review scope
2. if project-local standards are claimed, resolve the active project-local review set from `docs/project_standards/_registry.md` instead of scanning `docs/project_standards/` blindly
3. map each rule point to the rule objects it affects
4. run closure review first
5. run side-effect review second
6. grade every real problem by severity and blocking status
7. add background, trigger mechanism, impact scope, and repair suggestion to each finding
8. give an overall conclusion and the next action for the current review scope

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

### 6.2 Findings That Should Not Be Reported By Default

Do not report the following by default:

1. wording preference only
2. naming-style preference only
3. personal taste about section organization
4. speculative suggestions without side-effect evidence
5. overdesigned suggestions that add rule complexity without clear risk reduction
6. subjective nitpicks that cannot be attributed to closure, conflict, side effect, or ambiguity

## 7. Output Contract

The output should include:

1. review scope
2. overall conclusion
3. findings ordered by severity and blocking priority
4. for each finding:
   - what the problem is
   - why it happens
   - what it impacts
   - the minimal recommended fix
5. whether the current review passes or is blocked
6. the next action

## 8. Non-Goals

This flow does not:

1. review business-module behavior design
2. verify implementation alignment for a concrete module
3. replace `cand_check`, `cand_verify`, or `stable_verify`
4. execute reconciliation work in place of `shared_flow_reconcile`
5. treat unregistered files under `docs/project_standards/` as active governance inputs

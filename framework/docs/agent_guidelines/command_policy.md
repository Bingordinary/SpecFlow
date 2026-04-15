# Command Policy

## 1. Purpose

This file defines how standard commands work in this repository.

It answers four questions:

1. what a command is
2. which objects commands operate on
3. what each command is responsible for
4. how an agent should match and execute command-style requests

---

## 2. What A Command Is

A command is the standard workflow entry for the agent.

It is not a shell command, and it is not the business rule itself.

In plain words:

1. `Spec` is the truth.
2. `Command` is the action.

---

## 3. Objects Operated By Commands

Commands normally operate on these objects:

1. `stable`
2. `candidate`
3. `plan`
4. `check_result`
5. `verify_result`
6. `status`

Additional notes:

1. Here, `status` always means `docs/specs/_status.md`. It is a state index file, not a behavior source of truth, but every standard command must maintain it according to the rules.
2. `system_constraints` is the unique global system-constraint object. It is not one of the six ordinary command-process objects.
3. `shared appendix` is a shared supporting-truth object. It is not one of the six ordinary command-process objects and it is not an independent command target.
4. When a module command needs to judge the formal technical baseline, shared mechanisms, or global exceptions, and `docs/specs/system/stable/s_system_constraints.md` exists, that file must be read as an upstream constraint input.
5. `system_constraints` does not enter `docs/specs/_status.md` and does not produce its own `_check_result`, `_plans`, or `_verify_result`.
6. If a module needs to propose a new global constraint change, that proposal can only be written in the module's own `candidate`.
7. `s_system_constraints.md` may be created or updated only as a linked side product of module `cand_promote`.

---

## 4. Command Format

The standard command format is:

```text
{command}:{module}
```

Where:

1. `{command}` is a stable command name.
2. `{module}` is a formal module name, such as `module_example`.

Additional rules:

1. `{module}` must use the formal module name, not a file prefix.
2. `system_constraints` is not a legal command target. Do not write forms such as `spec_new:system_constraints` or `cand_check:system_constraints`.
3. `{module}` points only to objects formally recognized as modules by the mechanism. Appendix files, topic-expansion files, Prompt source templates, and similar supporting files are not legal command targets.
4. `shared appendix` is also not a legal command target. Do not write `cand_check:shared_xxx` or equivalent forms.
5. `shared_flow_reconcile` is not a standard module command in `{command}:{module}` form. Do not write `shared_flow_reconcile:module_xxx`.
6. `shared_extract_review` is also not a standard module command in `{command}:{module}` form.
7. `project_standard_create` is also not a standard module command in `{command}:{module}` form.
8. checkpoints and clarification actions are not standard commands in `{command}:{module}` form.
9. only the standard commands listed in Section 5 advance the normal module lifecycle.
10. Only for the first-version entry commands `spec_init:{module}` and `spec_new:{module}`, `{module}` may point to a new target that is not yet in `_status.md` but already has a clear, non-conflicting module name.
11. Outside that exception, if a file is not yet registered as an independent formal module in `_status.md`, it must not be treated as a `{module}` target just because its file name, path, or frontmatter looks module-like.

---

## 5. Standard Commands

### 5.1 Version Commands

1. `spec_init:{module}`
2. `stable_verify:{module}`
3. `spec_new:{module}`
4. `spec_fork:{module}`
5. `cand_promote:{module}`

### 5.2 Candidate Commands

1. `cand_check:{module}`
2. `cand_plan:{module}`
3. `cand_impl:{module}`
4. `cand_verify:{module}`

Additional requirements:

1. Any command index document for executors must list all standard commands above, including non-candidate-side commands such as `stable_verify`.
2. If a command index conflicts with this section, this section and the corresponding command file take precedence, and the drifted index should be corrected in the current task.
3. The default registry of entry-index documents is defined by `specflow/framework/docs/agent_guidelines/entry_index_registry.md`.

---

## 6. Responsibilities Of Each Command Type

### 6.1 Version Commands

Version commands create, open, or switch version layers.

1. `spec_init`
   - creates the first `stable` for a historical module
2. `stable_verify`
   - verifies whether current code still aligns with `stable`
   - outputs structured verification evidence and updates `_status.md`
3. `spec_new`
   - creates the first `candidate` for a new module
4. `spec_fork`
   - opens a new `candidate` from an existing `stable`
5. `cand_promote`
   - promotes the current `candidate` into the new `stable`
   - updates `s_system_constraints.md` when needed

### 6.2 Candidate Commands

Candidate commands move a candidate from design to implementation and then to promotion.

1. `cand_check`
   - checks whether the candidate is sufficiently closed to stably constrain implementation
   - checks whether `system_constraints_stable_ref` aligns with the current formal global baseline
   - runs Prompt Adequacy Review only when the current project has a registered `review_standard` with `surface=prompt_review` consumed by `cand_check`
   - may additionally consume registered project-local review standards according to `specflow/framework/docs/agent_guidelines/project_standards_policy.md`
   - if it passes, writes `_check_result/{module}.md` as the pass gate for the candidate chain
2. `cand_plan`
   - reads `_check_result/{module}.md`
   - creates or updates `_plans/{module}.md`
3. `cand_impl`
   - implements against the candidate Spec and `_plans/{module}.md` after confirming the pass gate is still valid
4. `cand_verify`
   - verifies whether implementation aligns with the current candidate after confirming the pass gate and plan are still valid
   - writes `_verify_result/{module}.md`

---

## 7. Default Lifecycle Order

Formal modules normally follow two main chains:

1. `stable` maintenance chain
   - `spec_init`
   - `stable_verify`
   - `spec_fork`
2. `candidate` upgrade chain
   - `spec_new`
   - `spec_fork`
   - `cand_check`
   - `cand_plan`
   - `cand_impl`
   - `cand_verify`
   - `cand_promote`

---

## 8. Gate Rules

The rules below are shared gates. Every command follows them by default:

1. Do not execute a command if its prerequisite self-checks have not passed.
2. Do not enter `cand_plan` before passing `cand_check`.
3. If there is no valid `_check_result/{module}.md` for the current candidate, do not enter `cand_plan`, `cand_impl`, or `cand_verify`.
4. If there is no valid `_plans/{module}.md`, do not enter `cand_impl` or `cand_verify`.
5. Do not execute `cand_promote` before `cand_verify` is complete and all blocking items are cleared.
6. When `Active Layer=stable`, if implementation changed but `stable_verify` has not been done, do not claim the code is still aligned with `stable`.
7. For modules with `Active Layer=stable`, do not claim the code still aligns with `stable` until `stable_verify` has confirmed that conclusion for the current implementation state.
8. `Next Command` is the default next permitted action. Do not skip past it unless an explicit rule allows it.
9. Process files are not valid just because they exist. Their bound Spec layer, Spec file, version references, fingerprints, and command-required fields must also match.
10. Every module candidate must explicitly record `system_constraints_stable_ref`.
11. If a module depends on Shared Appendix files at the current layer, it must also explicitly record `shared_appendix_refs`.
12. If `s_system_constraints.md` exists and the module candidate's `system_constraints_stable_ref` does not equal the current stable system-constraint version, the module's candidate-side process files become invalid and fall back to `cand_check`.
13. If `s_system_constraints.md` does not exist and the module candidate's `system_constraints_stable_ref` is not `none`, the module's candidate-side process files become invalid and fall back to `cand_check`.
14. If the Shared Appendix versions, bodies, or bindings referenced by `shared_appendix_refs` change, the module's candidate-side process files become invalid and fall back to `cand_check`.
15. If a stable-layer module's bound stable Shared Appendix changed, the module may no longer claim it still aligns with `stable` and falls back to `stable_verify`.
16. If a stable-layer module's current stable truth explicitly records `system_constraints_stable_ref` and that recorded reference no longer matches the current formal global baseline state, the module may no longer claim it still aligns with `stable` and falls back to `stable_verify`.
17. `cand_verify` does not manage an independent `system_constraints` state machine. It only verifies implementation against the current candidate system.
18. `cand_promote` must absorb closed global-constraint proposals into `docs/specs/system/stable/s_system_constraints.md` when promotion confirms those proposals are ready.
19. When `cand_plan`, `cand_impl`, or `cand_verify` reads `_check_result/{module}.md`, it must confirm both the required bindings and `decision=pass` plus `allow_next=true`.
20. When `cand_promote` reads `_verify_result/{module}.md`, it must confirm both the required bindings and `decision=pass`, `allow_next=true`, and `next_command=cand_promote`.
21. If the current project has an active registered Prompt review standard for the current target and the candidate fails Prompt Adequacy Review, it must not enter `cand_plan` and must not keep using an old pass gate.
22. `Prompt Adequacy Review` may return `n/a` when Prompt triggers were not hit, or when the current project has no active registered Prompt review standard for the current target.
23. When `cand_check` does not pass, it must not write a failed `_check_result/{module}.md`. If an old pass gate is no longer valid, delete it and keep or fall back `Next Command` to `cand_check`.
24. `cand_check` does not directly rewrite candidate truth by default. The only allowed automatic correction is a mechanical update of `system_constraints_stable_ref` when the candidate is still compatible with the current formal global baseline, or correction to `none` when no formal global baseline exists yet.
25. A blocking checkpoint is not a pass result and must not be treated as permission to continue to the next command.
26. When a command resumes after a checkpoint, it must re-judge the required bindings and gate conditions instead of assuming the checkpoint answer already fixed them.
27. Candidate-side fallback, blocking, and resume outputs must report the standardized `fallback_reason_code` defined by `specflow/framework/docs/agent_guidelines/candidate_handoff_contract.md` before any free-form explanation.
28. When `cand_verify` or `stable_verify` needs to judge whether `partial` or `not_checked` items may still support a narrower safe conclusion, it must use `specflow/framework/docs/agent_guidelines/downgrade_policy.md` instead of executor invention.
29. Commands must not consume project-local standards unless those standards are registered in `docs/project_standards/_registry.md` and the command explicitly supports that consumption surface.
30. Project-local standards may tighten or clarify framework baseline rules, but must not weaken them.

---

## 9. Command File Contract

Every command file must contain these core sections by default:

1. `Purpose`
2. `Scope`
3. `Preconditions`
4. `Procedure`
5. `Stop Conditions`
6. `Output Contract`
7. `Non-Goals`
An `Examples` section is optional and should be included only when it reduces command-entry ambiguity or clarifies a non-obvious boundary.

Additional requirements:

1. The command file must clearly state which object it operates on.
2. The command file must clearly state its upstream prerequisites.
3. The command file must clearly state its stop conditions.
4. It must not collapse the responsibilities of candidate truth, `_plans`, `_check_result`, and `_verify_result` into one object.
5. If the command consumes `system_constraints`, it must state that those are upstream constraints, not the command's primary output.
6. If the command consumes Shared Appendix files, it must state that they are shared truth objects bound in by a module, not independent command targets.
7. If the command file involves lifecycle closure, fallback, or cleanup, it must not invent an alternative set of top-level rules.
8. If the command file involves Prompt gates, it must clearly define:
   - trigger conditions
   - review dimensions
   - blocking standards
   - the priority between KV-cache-friendly ordering and semantic clarity
   - either directly in the command file or through one explicitly referenced centralized contract
9. If the command file writes back Prompt review results, it must define the minimum snapshot contract, either directly or through one explicitly referenced centralized contract, instead of leaving field meanings to executor invention.
10. If the command requires mandatory close-out work such as a git-history decision, it must explicitly reference the relevant governance rule instead of leaving that step to executor memory.
11. If the command may raise a checkpoint, it must define the allowed checkpoint types, trigger conditions, and resume rules.
12. If the command may fall back or block, it must define which standardized `fallback_reason_code` values it may emit instead of leaving fallback wording to executor invention.
13. If the command grades findings or deviations by severity, it must use one explicitly referenced centralized severity contract instead of redefining severity meanings locally.

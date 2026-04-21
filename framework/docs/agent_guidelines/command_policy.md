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
3. `shared contract` is a shared supporting-truth object. It is not one of the six ordinary command-process objects and it is not an independent `{command}:{module}` target.
4. When a module command needs to judge the formal technical baseline, shared mechanisms, or global exceptions, and `docs/specs/system/stable/s_system_constraints.md` exists, that file must be read as an upstream constraint input.
5. `system_constraints` does not enter `docs/specs/_status.md` and does not produce its own `_check_result`, `_plans`, or `_verify_result`.
6. If a module needs to propose a new global constraint change, that proposal can only be written in the module's own `candidate`.
7. `s_system_constraints.md` may be created or updated only as a linked side product of module `cand_promote`.
8. That proposal should be recorded as `system_constraints_change_proposal` inside the module candidate rather than as an independent system candidate file.

---

## 4. Command Format

The standard module command format is:

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
4. `shared contract` is also not a legal module command target. Do not write `cand_check:shared_xxx` or equivalent forms.
5. `shared_ops:{natural-language request}` is a user-facing shared-governance command entry, but it is not a `{command}:{module}` command.
6. `shared_new`, `shared_extract`, `shared_bind`, `shared_topology`, `shared_sync`, and `shared_escape` are internal shared-governance flow names, not direct user-facing commands.
7. `project_standard_create` is also not a standard module command in `{command}:{module}` form.
8. checkpoints and clarification actions are not standard commands in `{command}:{module}` form.
9. only the standard commands listed in Section 5 advance the normal module lifecycle.
10. Only for the first-version entry commands `spec_init:{module}` and `spec_new:{module}`, `{module}` may point to a new target that is not yet in `_status.md` but already has a clear, non-conflicting module name.
11. Outside that exception, if a file is not yet registered as an independent formal module in `_status.md`, it must not be treated as a `{module}` target just because its file name, path, or frontmatter looks module-like.

### 4.1 Shared Governance Entry

The user-facing shared-governance entry is:

```text
shared_ops:{natural-language request}
```

Rules:

1. `shared_ops` is the only preferred user-facing entry for shared-truth governance
2. it is intent-driven rather than object-name-driven
3. it routes into internal shared flows according to `specflow/framework/docs/agent_guidelines/shared_ops.md`
4. if routing cannot be stabilized safely, it must enter `shared_escape` and then checkpoint when required

### 4.2 Direct Implementation Request Gate

A direct implementation request means the user asks the executor to modify repo-tracked code or other implementation-side files without first entering a standard module command.

Rules:

1. every direct implementation request must be classified first through `specflow/framework/docs/agent_guidelines/implementation_change_policy.md`
2. the only legal classification results are:
   - `implementation_only`
   - `truth_writeback_required`
   - `boundary_unclear`
3. `implementation_only` means implementation may continue only inside the current `Active Layer`, current `Next Command`, and current verification obligations
4. `implementation_only` is not permission to skip lifecycle gates or to silently change truth later
5. `truth_writeback_required` means the smallest legal next step is truth-side writeback or command routing, not code modification
6. `boundary_unclear` means repository truth is not sufficient to safely start from code and must be treated exactly like `truth_writeback_required`
7. a direct implementation request is a gate, not a new command

The smallest legal next step is fixed as follows:

| Current situation | Smallest legal next step |
|---|---|
| brand-new module, user directly asks to write code | `spec_new:{module}` |
| existing `stable` module, and the requested change would alter formal behavior truth | `spec_fork:{module}` first, then write the new candidate truth before implementation |
| existing `candidate` module, and the requested change would alter current candidate truth | write back into the current candidate main file, required appendix truth, or required Shared Contract truth first, then rerun `cand_check:{module}` |
| request touches cross-module shared truth | `shared_ops:{natural-language request}` |
| `implementation_only`, target module has `Active Layer=stable` | implementation may continue only inside current stable truth; after code changes, `stable_verify:{module}` is required before stable alignment may be claimed again |
| `implementation_only`, target module has `Active Layer=candidate` and `_status.md` says `Next Command=cand_impl` | implementation may continue only under `cand_impl` semantics |
| `implementation_only`, target module has `Active Layer=candidate` and `_status.md` says any `Next Command` other than `cand_impl` | do not modify code; return to the currently recorded smallest legal next step first |

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
4. Shared governance is user-entered through `shared_ops:{natural-language request}` rather than being added as a second module-command chain.

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
11. If a module depends on Shared Contract files at the current layer, it must also explicitly record `shared_contract_refs` using the Shared Contract binding contract from `specflow/framework/docs/agent_guidelines/spec_policy.md` Section 6.1.
12. If `s_system_constraints.md` exists and the module candidate's `system_constraints_stable_ref` does not equal the current stable system-constraint version, the module's candidate-side process files become invalid and fall back to `cand_check`.
13. If `s_system_constraints.md` does not exist and the module candidate's `system_constraints_stable_ref` is not `none`, the module's candidate-side process files become invalid and fall back to `cand_check`.
14. If the effective module-local appendix truth explicitly referenced by the current-layer main Spec changes, the module's candidate-side process files become invalid and fall back to `cand_check`.
15. If the effective Shared Contract truth resolved from `shared_contract_refs` under that binding contract changes, the module's candidate-side process files become invalid and fall back to `cand_check`.
16. A `bound_modules`-only delta does not by itself invalidate candidate-side process files, because `bound_modules` is declarative metadata rather than the module's formal binding source. Report governance drift instead.
17. If a stable-layer module's explicitly referenced stable appendix truth changes, the module may no longer claim it still aligns with `stable` and falls back to `stable_verify`.
18. If a stable-layer module's bound stable Shared Contract changed, the module may no longer claim it still aligns with `stable` and falls back to `stable_verify`.
   - exception: when the module is the current target of a still-closing stable-landing round and that same round wrote the module's current stable truth together with its current stable Shared Contract binding, keep that just-landed module under the active stable-landing command rather than treating it as a stale stable-alignment claim
   - this exception exists at minimum for `spec_init` and `cand_promote`
19. If a stable-layer module's current stable truth explicitly records `system_constraints_stable_ref` and that recorded reference no longer matches the current formal global baseline state, the module may no longer claim it still aligns with `stable` and falls back to `stable_verify`.
20. `cand_verify` does not manage an independent `system_constraints` state machine. It only verifies implementation against the current candidate system.
21. `cand_promote` must absorb closed global-constraint proposals into `docs/specs/system/stable/s_system_constraints.md` when promotion confirms those proposals are ready.
22. When `cand_plan`, `cand_impl`, or `cand_verify` reads `_check_result/{module}.md`, it must confirm both the required bindings and `decision=pass` plus `allow_next=true`.
23. When `cand_promote` reads `_verify_result/{module}.md`, it must confirm both the required bindings and `decision=pass`, `allow_next=true`, and `next_command=cand_promote`.
24. When `cand_check` does not pass, it must not write a failed `_check_result/{module}.md`. If an old pass gate is no longer valid, delete it and keep or fall back `Next Command` to `cand_check`.
25. `cand_check` does not directly rewrite candidate truth by default. The only allowed automatic correction is a mechanical update of `system_constraints_stable_ref` when the candidate is still compatible with the current formal global baseline, or correction to `none` when no formal global baseline exists yet.
26. A blocking checkpoint is not a pass result and must not be treated as permission to continue to the next command.
27. A formal pass gate, formal verification pass, or lifecycle-state advance may be produced only by a new independent full-scope run of the corresponding command.
    - here, `full-scope run` means rerunning that command's mandatory read scope, mandatory review surfaces, gate checks, and output contract for the current target rather than rechecking only one repaired finding or one changed truth fragment
    - a narrowed review, local confirmation, repair-side reassessment, or any other scoped follow-up does not count as a `full-scope run`
28. The identity of that full-scope run is determined by command routing, not by literal command syntax alone. A user may enter the run through explicit command form or through a new natural-language request that is correctly resolved to that command.
    - after a prior run of that same command ended with a non-pass result other than a resumable checkpoint, a later natural-language request counts as a new authoritative rerun only when the rerun intent is explicit enough to distinguish "rerun the command now" from "repair truth", "continue follow-up work", or "recheck one finding"
    - for that post-non-pass case, generic natural-language requests such as "fix it", "continue", "close this up", or equivalent repair-oriented wording do not by themselves authorize a new authoritative rerun
    - if that rerun intent is not explicit enough, command routing must keep the request on the non-authoritative repair or follow-up path and must not treat it as a fresh full-scope run
29. After a command ends with any non-pass result other than a checkpoint that the command file explicitly allows to stay resumable, any later truth repair, repair-side reassessment, or scoped follow-up review is non-authoritative for lifecycle progression.
30. Rule 29 means that such follow-up work may report only what was rechecked inside its actual scope. It must not be treated as a formal rerun of the prior command, must not write a new pass gate, and must not advance `_status.md`.
31. When a command resumes after a checkpoint, it must re-judge the required bindings and gate conditions instead of assuming the checkpoint answer already fixed them.
32. Candidate-side fallback, blocking, and resume outputs must report the standardized `fallback_reason_code` defined by `specflow/framework/docs/agent_guidelines/candidate_handoff_contract.md` before any free-form explanation.
33. When `cand_verify` or `stable_verify` needs to judge whether `partial` or `not_checked` items may still support a narrower safe conclusion, it must use `specflow/framework/docs/agent_guidelines/downgrade_policy.md` instead of executor invention.
34. Commands must not consume project-local standards unless those standards are registered in `docs/project_standards/_registry.md` and the command explicitly supports that consumption surface.
35. Project-local standards may tighten or clarify framework baseline rules, but must not weaken them.
36. A command must not treat the presence of a project-local standard as permission to skip its framework-baseline review.
37. When a command consumes project-local standards, it must first complete the framework-baseline judgment and then merge project-local results only on the command-defined supported `surface`.
38. The final command conclusion must still stay inside the framework-defined result set of that command.
39. A downstream command must not consume a project-side extension field unless that downstream command explicitly declares that consumption contract.
40. Shared-governance requests must enter through `shared_ops:{natural-language request}` rather than by asking the user to pre-select an internal shared flow.
41. A direct implementation request must be classified through `specflow/framework/docs/agent_guidelines/implementation_change_policy.md` before repo-tracked code is modified.
42. `implementation_only` does not bypass `Next Command`.
43. If a direct implementation request is classified as `truth_writeback_required` or `boundary_unclear`, the executor must not modify code before the required truth-side writeback or routing step has completed.
44. For `implementation_only` on `Active Layer=candidate`, code modification is allowed only when `_status.md` currently says `Next Command=cand_impl`.
45. For `implementation_only` on `Active Layer=stable`, code modification may proceed only inside current stable truth, and `stable_verify` is required before stable alignment may be claimed again.

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
6. If the command consumes Shared Contract files, it must state that they are shared truth objects bound in by a module, not independent command targets.
7. If the command file involves lifecycle closure, fallback, or cleanup, it must not invent an alternative set of top-level rules.
8. If the command consumes project-local standards, it must clearly define:
   - the supported `surface` names owned by that command
   - the meaning and trigger condition of each supported `surface`
   - that consumption is optional and depends on registered active entries
   - which registered entry shapes it may consume
   - which part of the command decision surface those standards may tighten or clarify
   - whether those standards affect pass, fallback, or output write-back
   - how those project-local results merge into the command's framework-baseline conclusion
   - which project-side extension fields, if any, are allowed and what their boundary is against framework fixed fields
9. If the command requires mandatory close-out work such as a git-history decision, it must explicitly reference the relevant governance rule instead of leaving that step to executor memory.
10. If the command may raise a checkpoint, it must define the allowed checkpoint types, trigger conditions, and resume rules.
11. If the command may fall back or block, it must define which standardized `fallback_reason_code` values it may emit instead of leaving fallback wording to executor invention.
12. If the command grades findings or deviations by severity, it must use one explicitly referenced centralized severity contract instead of redefining severity meanings locally.
13. If the command may advance `_status.md` to a later lifecycle step, it must explicitly state that such advancement inherits the centralized authoritative-run and non-authoritative-follow-up rules from Section 8 Rules 27-30 instead of redefining a second advancement contract locally.
14. Every user-facing standard command final response must begin with a `user-facing close-out block`.
15. That `user-facing close-out block` must appear before process detail, evidence matrices, file inventories, or git close-out detail.
16. The `user-facing close-out block` must include all of the following fixed semantic slots in this order:
   - `round conclusion`
   - `current state`
   - `next step`
   - `why this next step`
   - `next-stage entry gap`
17. The slot names above define semantic meaning and order, not mandatory literal surface labels.
18. Executors and hosts may localize or restyle the displayed labels if all of the following still hold:
   - the slot order stays unchanged
   - each slot keeps the same meaning
   - the rendered labels remain easy for the user to distinguish
19. For standard module commands, `current state` must explicitly report the command-owned lifecycle state written back in `docs/specs/_status.md`, including at minimum:
   - `Active Layer`
   - `Next Command`
20. `next step` must name the smallest legal next step for the current result rather than a broad later-phase suggestion.
21. `why this next step` must explain the blocking fact, gate fact, or completion fact in plain user-facing language rather than only repeating the command name or `_status.md` field.
22. If `Next Command` remains the same command that just ran, `why this next step` must explicitly state that the round is not stalled and must name the concrete unfinished closure surface that keeps the workflow on the current command.
23. `next-stage entry gap` has one fixed boundary meaning:
   - it reports the entry condition for the first later lifecycle command that is different from the command that just ran
   - if that later different command is already the current `next step`, the slot must explicitly say that the later-stage entry condition is already satisfied
   - if the workflow is still staying on the same command, the slot must name what still blocks entry into that later different command
24. If the command result is blocked, stopped at a checkpoint, or waiting on a named prerequisite condition, the same `user-facing close-out block` must also include `resume signal`.
